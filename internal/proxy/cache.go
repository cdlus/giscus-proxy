package proxy

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (p *Proxy) cacheKey(r *http.Request) string {
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
