package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"giscus-proxy/internal/cache"
	"giscus-proxy/internal/config"
	"giscus-proxy/internal/proxy"
)

func main() {
	client := &http.Client{Timeout: 25 * time.Second}
	p := proxy.New(proxy.Config{
		Client: client,
		Cache:  cache.NewMemoryCache(512),
	})

	mux := http.NewServeMux()
	p.Register(mux)

	addr := strings.TrimSpace(os.Getenv("ADDR"))
	if addr == "" {
		host := config.GetEnv("HOST", "0.0.0.0")
		port := config.GetEnv("PORT", "8080")
		port = strings.TrimPrefix(port, ":")
		addr = host + ":" + port
	}

	log.SetOutput(os.Stdout)

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          log.New(os.Stdout, "", 0),
	}

	publicURL := config.DerivePublicURL(addr, config.GetEnv("HOST", ""), config.GetEnv("PORT", ""))
	log.Printf("giscus proxy listening: bind=%s url=%s", addr, publicURL)
	log.Fatal(srv.ListenAndServe())
}
