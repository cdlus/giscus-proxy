package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	upstreamOrigin = "https://giscus.app"
	widgetPath     = "/en/widget"
)

var httpClient = &http.Client{Timeout: 25 * time.Second}

// ---------- logging helpers ----------

type statusWriter struct {
	http.ResponseWriter
	status  int
	written int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.written += n
	return n, err
}

// ---------- simple response cache ----------

type cacheEntry struct {
	status  int
	headers http.Header
	body    []byte
	expires time.Time
}

type memoryCache struct {
	mu         sync.RWMutex
	data       map[string]cacheEntry
	maxEntries int
}

func newMemoryCache(maxEntries int) *memoryCache {
	return &memoryCache{data: make(map[string]cacheEntry), maxEntries: maxEntries}
}

func (c *memoryCache) Get(key string) (cacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.data[key]
	if !ok {
		return cacheEntry{}, false
	}
	if time.Now().After(v.expires) {
		return cacheEntry{}, false
	}
	return v, true
}

func (c *memoryCache) Set(key string, val cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.data) >= c.maxEntries {
		for k := range c.data { // naive eviction
			delete(c.data, k)
			break
		}
	}
	c.data[key] = val
}

var respCache = newMemoryCache(512)

var cacheHeaderKeys = []string{"Content-Type", "Content-Encoding", "Cache-Control", "ETag", "Last-Modified", "Vary"}

func cacheKey(r *http.Request) string {
	return r.Method + " " + r.URL.RequestURI() + " ae=" + strings.TrimSpace(r.Header.Get("Accept-Encoding"))
}

func parseMaxAge(h http.Header) (time.Duration, bool) {
	cc := h.Get("Cache-Control")
	if cc == "" {
		return 0, false
	}
	parts := strings.Split(cc, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(strings.ToLower(p), "max-age=") {
			v := strings.TrimSpace(p[len("max-age="):])
			if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
				return time.Duration(secs) * time.Second, true
			}
		}
	}
	return 0, false
}

// pretty logging helpers

func fmtDur(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%4dms", d.Milliseconds())
	}
	sec := float64(d) / float64(time.Second)
	return fmt.Sprintf("%6.2fs", sec)
}

func logLine(kind, method, path string, status, bytes int, dur time.Duration, cacheState, target string) {
	if cacheState == "" {
		cacheState = "-"
	}
	log.Printf("%-6s method=%-4s status=%3d bytes=%8d dur=%9s cache=%-10s path=%s target=%s",
		kind, method, status, bytes, fmtDur(dur), cacheState, path, target)
}

func writeCORS(h http.ResponseWriter) {
	h.Header().Set("Access-Control-Allow-Origin", "*")
	h.Header().Set("Vary", "Origin")
	h.Header().Set("Access-Control-Allow-Methods", "GET,HEAD,OPTIONS")
	h.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization,Accept")
}

func copyIf(dst, src http.Header, keys ...string) {
	for _, k := range keys {
		if v := src.Get(k); v != "" {
			dst.Set(k, v)
		}
	}
}

func getEnv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func ensureURL(v string, defaultScheme string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return v
	}
	if defaultScheme == "" {
		defaultScheme = "https"
	}
	return defaultScheme + "://" + v
}

func derivePublicURL(bindAddr, host, port string) string {
	if u := ensureURL(os.Getenv("PUBLIC_URL"), ""); u != "" {
		return u
	}
	if u := ensureURL(os.Getenv("RAILWAY_PUBLIC_DOMAIN"), "https"); u != "" {
		return u
	}
	if u := ensureURL(os.Getenv("RAILWAY_URL"), ""); u != "" {
		return u
	}

	// Fallback to local composition
	p := strings.TrimSpace(port)
	h := strings.TrimSpace(host)
	if p == "" {
		b := bindAddr
		if strings.HasPrefix(b, ":") {
			p = strings.TrimPrefix(b, ":")
		} else if i := strings.LastIndex(b, ":"); i != -1 {
			p = b[i+1:]
		}
	}
	if h == "" {
		b := bindAddr
		if strings.HasPrefix(b, ":") || b == "" {
			h = "localhost"
		} else if i := strings.LastIndex(b, ":"); i != -1 {
			h = b[:i]
		}
	}
	if h == "0.0.0.0" || h == "::" || h == "[::]" || h == "" {
		h = "localhost"
	}
	if p == "" {
		p = "8080"
	}
	return "http://" + h + ":" + p
}

func decompressIfNeeded(h http.Header, body io.ReadCloser) (io.ReadCloser, func(), error) {
	enc := strings.ToLower(strings.TrimSpace(h.Get("Content-Encoding")))
	switch enc {
	case "", "identity":
		return body, func() {}, nil
	case "gzip":
		zr, err := gzip.NewReader(body)
		if err != nil {
			return nil, func() {}, err
		}
		return zr, func() { _ = zr.Close(); _ = body.Close() }, nil
	default:
		return nil, func() {}, fmt.Errorf("unsupported content-encoding: %s", enc)
	}
}

type replacer struct {
	useRegex bool
	from     string
	fromRE   *regexp.Regexp
	to       string
}

func parseReplacers(q url.Values) ([]replacer, error) {
	vals := q["rep"]
	if len(vals) == 0 {
		return nil, nil
	}
	var out []replacer
	for _, raw := range vals {
		parts := strings.SplitN(raw, "=>", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("bad rep value %q (use LEFT=>RIGHT)", raw)
		}
		left, right := parts[0], parts[1]
		if strings.HasPrefix(left, "re:") {
			pat := left[len("re:"):]
			re, err := regexp.Compile(pat)
			if err != nil {
				return nil, fmt.Errorf("regex compile failed for %q: %w", pat, err)
			}
			out = append(out, replacer{useRegex: true, fromRE: re, to: right})
		} else {
			out = append(out, replacer{from: left, to: right})
		}
	}
	return out, nil
}

func applyReplacements(b []byte, reps []replacer) []byte {
	if len(reps) == 0 {
		return b
	}
	s := string(b)
	for _, r := range reps {
		if r.useRegex {
			s = r.fromRE.ReplaceAllString(s, r.to)
		} else {
			s = strings.ReplaceAll(s, r.from, r.to)
		}
	}
	return []byte(s)
}

func widgetFooterSwap(b []byte) []byte {
	s := string(b)
	s = strings.ReplaceAll(s, "– powered by \\u003ca\\u003egiscus\\u003c/a\\u003e", "")
	s = strings.ReplaceAll(s, "– powered by <a>giscus</a>", "")
	s = strings.ReplaceAll(s, "- powered by <a>giscus</a>", "")
	return []byte(s)
}

func handleWidget(w http.ResponseWriter, r *http.Request) {
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	start := time.Now()
	var target string
	defer func() {
		logLine("widget", r.Method, r.URL.RequestURI(), sw.status, sw.written, time.Since(start), "", target)
	}()
	w = sw
	if r.Method == http.MethodOptions {
		writeCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := r.URL.Query()
	reps, err := parseReplacers(q)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tq := url.Values{}
	for k, vs := range q {
		if k == "rep" {
			continue
		}
		for _, v := range vs {
			tq.Add(k, v)
		}
	}
	target = upstreamOrigin + widgetPath
	if enc := tq.Encode(); enc != "" {
		target += "?" + enc
	}

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("User-Agent", "giscus-wrap/clean-1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	writeCORS(w)
	copyIf(w.Header(), resp.Header, "Content-Type")

	body, clean, decErr := decompressIfNeeded(resp.Header, resp.Body)
	if decErr != nil {
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	defer clean()

	bin, err := io.ReadAll(body)
	if err != nil {
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write([]byte(fmt.Sprintf("<!-- read body failed: %v -->", err)))
		return
	}

	bin = applyReplacements(bin, reps)
	bin = widgetFooterSwap(bin)

	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = w.Write(bin)
	}
}

// /api/* -> upstream passthrough (NO replacements)
func handlePassthrough(w http.ResponseWriter, r *http.Request) {
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	start := time.Now()
	var target string
	cacheState := "BYPASS"
	defer func() {
		logLine("pass", r.Method, r.URL.RequestURI(), sw.status, sw.written, time.Since(start), cacheState, target)
	}()
	w = sw

	if r.Method == http.MethodOptions {
		writeCORS(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Build upstream URL, forwarding path and query as-is
	target = upstreamOrigin + r.URL.Path
	if raw := r.URL.RawQuery; raw != "" {
		target += "?" + raw
	}

	// Simple in-memory cache for GET/HEAD of uncompressed responses
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if ent, ok := respCache.Get(cacheKey(r)); ok {
			for _, k := range cacheHeaderKeys {
				if v := ent.headers.Get(k); v != "" {
					w.Header().Set(k, v)
				}
			}
			w.WriteHeader(ent.status)
			if r.Method == http.MethodGet {
				_, _ = w.Write(ent.body)
			}
			cacheState = "HIT"
			return
		}
	}

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
		return
	}
	if ae := r.Header.Get("Accept-Encoding"); ae != "" {
		req.Header.Set("Accept-Encoding", ae)
	}
	req.Header.Set("Accept", "*/*")
	req.Header.Set("User-Agent", "giscus-wrap/clean-1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	writeCORS(w)

	// Attempt cacheable path for GET when body is not compressed
	enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	if r.Method == http.MethodGet && (enc == "" || enc == "identity") && resp.StatusCode == http.StatusOK {
		bin, err := io.ReadAll(resp.Body)
		if err == nil {
			copyIf(w.Header(), resp.Header, cacheHeaderKeys...)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(bin)

			if ttl, ok := parseMaxAge(resp.Header); ok {
				h := http.Header{}
				for _, k := range cacheHeaderKeys {
					if v := resp.Header.Get(k); v != "" {
						h.Set(k, v)
					}
				}
				respCache.Set(cacheKey(r), cacheEntry{status: resp.StatusCode, headers: h, body: bin, expires: time.Now().Add(ttl)})
				cacheState = "MISS:cached"
				return
			}
		}
		copyIf(w.Header(), resp.Header, cacheHeaderKeys...)
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(bin)
		cacheState = "MISS"
		return
	}

	// Non-cacheable path or HEAD/other methods: stream
	copyIf(w.Header(), resp.Header, cacheHeaderKeys...)
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, resp.Body)
	}
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/widget", handleWidget)
	mux.HandleFunc("/en/widget", handleWidget)
	mux.HandleFunc("/", handlePassthrough)

	addr := ""
	if v := strings.TrimSpace(os.Getenv("ADDR")); v != "" {
		addr = v
	} else {
		host := getEnv("HOST", "0.0.0.0")
		port := getEnv("PORT", "8080")
		port = strings.TrimPrefix(port, ":")
		addr = host + ":" + port
	}
	// ensure logs go to stdout so PaaS platforms don't mark them as errors
	log.SetOutput(os.Stdout)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          log.New(os.Stdout, "", 0),
	}

	publicURL := derivePublicURL(addr, getEnv("HOST", ""), getEnv("PORT", ""))
	log.Printf("giscus wrapper listening: bind=%s url=%s", addr, publicURL)
	log.Fatal(srv.ListenAndServe())
}
