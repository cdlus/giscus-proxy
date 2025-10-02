package proxy

import (
	"log"
	"net/http"
	"time"

	"giscus-proxy/internal/cache"
)

// HTTPClient represents the subset of *http.Client used by the proxy.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config provides all the dependencies required to build a Proxy.
type Config struct {
	UpstreamOrigin   string
	WidgetSourcePath string
	WidgetPaths      []string
	CacheHeaders     []string
	Client           HTTPClient
	Cache            cache.Cache
	Logger           *log.Logger
}

// Proxy coordinates the handlers that proxy traffic to giscus.
type Proxy struct {
	upstreamOrigin   string
	widgetSourcePath string
	widgetPaths      []string
	cacheHeaders     []string
	client           HTTPClient
	cache            cache.Cache
	logger           *log.Logger
}

// New constructs a Proxy from the provided configuration, applying sensible defaults.
func New(cfg Config) *Proxy {
	p := &Proxy{
		upstreamOrigin:   cfg.UpstreamOrigin,
		widgetSourcePath: cfg.WidgetSourcePath,
		widgetPaths:      append([]string(nil), cfg.WidgetPaths...),
		cacheHeaders:     append([]string(nil), cfg.CacheHeaders...),
		client:           cfg.Client,
		cache:            cfg.Cache,
		logger:           cfg.Logger,
	}

	if p.upstreamOrigin == "" {
		p.upstreamOrigin = "https://giscus.app"
	}
	if p.widgetSourcePath == "" {
		p.widgetSourcePath = "/en/widget"
	}
	if len(p.widgetPaths) == 0 {
		p.widgetPaths = []string{"/widget", "/en/widget"}
	}
	if len(p.cacheHeaders) == 0 {
		p.cacheHeaders = []string{"Content-Type", "Content-Encoding", "Cache-Control", "ETag", "Last-Modified", "Vary"}
	}
	if p.client == nil {
		p.client = &http.Client{Timeout: 25 * time.Second}
	}
	if p.logger == nil {
		p.logger = log.Default()
	}

	return p
}

// Register attaches the proxy handlers to the provided mux.
func (p *Proxy) Register(mux *http.ServeMux) {
	for _, path := range p.widgetPaths {
		mux.HandleFunc(path, p.handleWidget)
	}
	mux.HandleFunc("/", p.handlePassthrough)
}

func (p *Proxy) logf(format string, args ...any) {
	if p.logger == nil {
		log.Printf(format, args...)
		return
	}
	p.logger.Printf(format, args...)
}
