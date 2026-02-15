package cache

import (
	"testing"
	"time"
)

func TestTTLCache(t *testing.T) {
	tests := []struct {
		name      string
		ttl       time.Duration
		key       string
		value     string
		sleep     time.Duration
		wantFound bool
		wantValue string
	}{
		{
			name:      "hit within TTL",
			ttl:       1 * time.Second,
			key:       "k1",
			value:     "v1",
			sleep:     0,
			wantFound: true,
			wantValue: "v1",
		},
		{
			name:      "miss after TTL",
			ttl:       1 * time.Millisecond,
			key:       "k2",
			value:     "v2",
			sleep:     5 * time.Millisecond,
			wantFound: false,
		},
		{
			name:      "miss for unknown key",
			ttl:       1 * time.Second,
			key:       "unknown",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New[string](tt.ttl)
			if tt.value != "" {
				c.Set(tt.key, tt.value)
			}
			if tt.sleep > 0 {
				time.Sleep(tt.sleep)
			}
			got, found := c.Get(tt.key)
			if found != tt.wantFound {
				t.Errorf("Get(%q) found = %v, want %v", tt.key, found, tt.wantFound)
			}
			if found && got != tt.wantValue {
				t.Errorf("Get(%q) = %v, want %v", tt.key, got, tt.wantValue)
			}
		})
	}
}

func TestTTLCacheInvalidate(t *testing.T) {
	c := New[string](1 * time.Minute)
	c.Set("k", "v")
	if _, ok := c.Get("k"); !ok {
		t.Fatal("expected hit before invalidate")
	}
	c.Invalidate("k")
	if _, ok := c.Get("k"); ok {
		t.Fatal("expected miss after invalidate")
	}
}

func TestTTLCacheStats(t *testing.T) {
	c := New[string](1 * time.Minute)
	c.Set("k", "v")
	c.Get("k")      // hit
	c.Get("missing") // miss

	hits, misses := c.Stats()
	if hits != 1 {
		t.Errorf("hits = %d, want 1", hits)
	}
	if misses != 1 {
		t.Errorf("misses = %d, want 1", misses)
	}
}
