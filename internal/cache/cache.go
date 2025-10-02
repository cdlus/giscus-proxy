package cache

import (
	"net/http"
	"sync"
	"time"
)

// Entry represents a cached HTTP response.
type Entry struct {
	Status  int
	Headers http.Header
	Body    []byte
	Expires time.Time
}

// Cache defines the behaviour required for storing HTTP responses.
type Cache interface {
	Get(key string) (Entry, bool)
	Set(key string, entry Entry)
}

// MemoryCache is a simple in-memory implementation of Cache.
type MemoryCache struct {
	mu         sync.RWMutex
	data       map[string]Entry
	maxEntries int
}

// NewMemoryCache constructs a MemoryCache limited to the provided number of entries.
func NewMemoryCache(maxEntries int) *MemoryCache {
	return &MemoryCache{data: make(map[string]Entry), maxEntries: maxEntries}
}

// Get retrieves a cache entry if present and not expired.
func (c *MemoryCache) Get(key string) (Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data[key]
	if !ok {
		return Entry{}, false
	}
	if time.Now().After(entry.Expires) {
		return Entry{}, false
	}
	return entry, true
}

// Set stores a cache entry, evicting an arbitrary entry when capacity is reached.
func (c *MemoryCache) Set(key string, entry Entry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.data) >= c.maxEntries {
		for k := range c.data {
			delete(c.data, k)
			break
		}
	}
	c.data[key] = entry
}

var _ Cache = (*MemoryCache)(nil)
