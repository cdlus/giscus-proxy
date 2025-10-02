package proxy

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"giscus-proxy/internal/cache"
)

func (p *Proxy) handlePassthrough(w http.ResponseWriter, r *http.Request) {
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	start := time.Now()
	var target string
	cacheState := "BYPASS"
	defer func() {
		p.logLine("pass", r.Method, r.URL.RequestURI(), sw.status, sw.written, time.Since(start), cacheState, target)
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

	target = p.upstreamOrigin + r.URL.Path
	if raw := r.URL.RawQuery; raw != "" {
		target += "?" + raw
	}

	if p.cache != nil && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
		if ent, ok := p.cache.Get(p.cacheKey(r)); ok {
			for _, k := range p.cacheHeaders {
				if v := ent.Headers.Get(k); v != "" {
					w.Header().Set(k, v)
				}
			}
			w.WriteHeader(ent.Status)
			if r.Method == http.MethodGet {
				_, _ = w.Write(ent.Body)
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
	req.Header.Set("User-Agent", "giscus-proxy/clean-1.0")

	resp, err := p.client.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	writeCORS(w)

	enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	if p.cache != nil && r.Method == http.MethodGet && (enc == "" || enc == "identity") && resp.StatusCode == http.StatusOK {
		bin, err := io.ReadAll(resp.Body)
		if err == nil {
			copyIf(w.Header(), resp.Header, p.cacheHeaders...)
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(bin)

			if ttl, ok := parseMaxAge(resp.Header); ok {
				h := http.Header{}
				for _, k := range p.cacheHeaders {
					if v := resp.Header.Get(k); v != "" {
						h.Set(k, v)
					}
				}
				p.cache.Set(p.cacheKey(r), cache.Entry{Status: resp.StatusCode, Headers: h, Body: bin, Expires: time.Now().Add(ttl)})
				cacheState = "MISS:cached"
				return
			}
		}
		copyIf(w.Header(), resp.Header, p.cacheHeaders...)
		w.WriteHeader(resp.StatusCode)
		if err == nil {
			_, _ = w.Write(bin)
		}
		cacheState = "MISS"
		return
	}

	copyIf(w.Header(), resp.Header, p.cacheHeaders...)
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		_, _ = io.Copy(w, resp.Body)
	}
}
