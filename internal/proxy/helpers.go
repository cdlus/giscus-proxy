package proxy

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

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

func fmtDur(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%4dms", d.Milliseconds())
	}
	sec := float64(d) / float64(time.Second)
	return fmt.Sprintf("%6.2fs", sec)
}

func (p *Proxy) logLine(kind, method, path string, status, bytes int, dur time.Duration, cacheState, target string) {
	if cacheState == "" {
		cacheState = "-"
	}
	p.logf("%-6s method=%-4s status=%3d bytes=%8d dur=%9s cache=%-10s path=%s target=%s",
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
