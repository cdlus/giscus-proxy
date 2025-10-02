package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func (p *Proxy) handleWidget(w http.ResponseWriter, r *http.Request) {
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	start := time.Now()
	var target string
	defer func() {
		p.logLine("widget", r.Method, r.URL.RequestURI(), sw.status, sw.written, time.Since(start), "", target)
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
	target = p.upstreamOrigin + p.widgetSourcePath
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
	req.Header.Set("User-Agent", "giscus-proxy/clean-1.0")

	resp, err := p.client.Do(req)
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
