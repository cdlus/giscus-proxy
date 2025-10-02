package handler

import (
	"net/http"
	"time"

	"giscus-proxy/internal/cache"
	"giscus-proxy/internal/proxy"
)

var defaultHandler http.Handler

func init() {
	p := proxy.New(proxy.Config{
		Client: &http.Client{Timeout: 25 * time.Second},
		Cache:  cache.NewMemoryCache(256),
	})
	defaultHandler = p.Handler()
}

// Handler is the entry point for Vercel's Go runtime.
func Handler(w http.ResponseWriter, r *http.Request) {
	defaultHandler.ServeHTTP(w, r)
}
