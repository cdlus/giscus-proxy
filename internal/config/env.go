package config

import (
	"os"
	"strings"
)

// GetEnv returns the trimmed value of an environment variable or a default when unset.
func GetEnv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

// EnsureURL normalises an input into a URL, applying a default scheme when necessary.
func EnsureURL(v, defaultScheme string) string {
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

// DerivePublicURL attempts to build a public URL for the service based on environment hints.
func DerivePublicURL(bindAddr, host, port string) string {
	if u := EnsureURL(os.Getenv("PUBLIC_URL"), ""); u != "" {
		return u
	}
	if u := EnsureURL(os.Getenv("RAILWAY_PUBLIC_DOMAIN"), "https"); u != "" {
		return u
	}
	if u := EnsureURL(os.Getenv("RAILWAY_URL"), ""); u != "" {
		return u
	}

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
