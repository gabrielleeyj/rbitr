package cache

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Entry holds a cached value with an expiry time.
type Entry[V any] struct {
	Value     V
	ExpiresAt time.Time
}

// TTLCache is a generic in-memory cache with per-key TTL.
type TTLCache[V any] struct {
	mu      sync.RWMutex
	entries map[string]Entry[V]
	ttl     time.Duration
	hits    atomic.Int64
	misses  atomic.Int64
}

// New creates a TTLCache with the given default TTL.
func New[V any](ttl time.Duration) *TTLCache[V] {
	return &TTLCache[V]{
		entries: make(map[string]Entry[V]),
		ttl:     ttl,
	}
}

// Get retrieves a value by key. Returns the value and true if found and not expired.
func (c *TTLCache[V]) Get(key string) (V, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok || time.Now().After(entry.ExpiresAt) {
		c.misses.Add(1)
		var zero V
		return zero, false
	}
	c.hits.Add(1)
	return entry.Value, true
}

// Set stores a value with the default TTL.
func (c *TTLCache[V]) Set(key string, value V) {
	c.mu.Lock()
	c.entries[key] = Entry[V]{
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
	c.mu.Unlock()
}

// Invalidate removes a specific key.
func (c *TTLCache[V]) Invalidate(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// InvalidatePrefix removes all entries whose keys start with the provided prefix.
func (c *TTLCache[V]) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	for key := range c.entries {
		if strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
		}
	}
	c.mu.Unlock()
}

// Stats returns hit and miss counts.
func (c *TTLCache[V]) Stats() (hits, misses int64) {
	return c.hits.Load(), c.misses.Load()
}
