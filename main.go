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
	"strings"
	"time"
)

const (
	upstreamOrigin = "https://giscus.app"
	widgetPath     = "/en/widget"
)

var httpClient = &http.Client{Timeout: 25 * time.Second}

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
	target := upstreamOrigin + widgetPath
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
	target := upstreamOrigin + r.URL.Path
	if raw := r.URL.RawQuery; raw != "" {
		target += "?" + raw
	}

	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		http.Error(w, "failed to build upstream request", http.StatusInternalServerError)
		return
	}
	// Pass client's encodings through since we won't modify the body here
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
	copyIf(w.Header(), resp.Header, "Content-Type", "Content-Encoding", "Cache-Control", "ETag", "Last-Modified", "Vary")

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
	log.Printf("giscus wrapper listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
