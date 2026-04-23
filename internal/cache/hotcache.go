package cache

import (
	"strings"
	"sync"
	"time"
)

type entry struct {
	value   interface{}
	expires time.Time
}

// TTLCache is a simple in-memory cache with per-entry TTL, backed by sync.Map.
type TTLCache struct {
	store sync.Map
	ttl   time.Duration
}

// NewTTLCache creates a cache where entries expire after the given duration.
func NewTTLCache(ttl time.Duration) *TTLCache {
	return &TTLCache{ttl: ttl}
}

// Get returns the cached value if it exists and has not expired.
func (c *TTLCache) Get(key string) (interface{}, bool) {
	v, ok := c.store.Load(key)
	if !ok {
		return nil, false
	}
	e := v.(*entry)
	if time.Now().After(e.expires) {
		c.store.Delete(key)
		return nil, false
	}
	return e.value, true
}

// Set stores a value with the default TTL.
func (c *TTLCache) Set(key string, value interface{}) {
	c.store.Store(key, &entry{value: value, expires: time.Now().Add(c.ttl)})
}

// Delete removes a cached entry.
func (c *TTLCache) Delete(key string) {
	c.store.Delete(key)
}

// DeleteByPrefix removes all entries whose key starts with the given prefix.
func (c *TTLCache) DeleteByPrefix(prefix string) {
	c.store.Range(func(key, _ interface{}) bool {
		if strings.HasPrefix(key.(string), prefix) {
			c.store.Delete(key)
		}
		return true
	})
}

// PurgeAll removes all entries.
func (c *TTLCache) PurgeAll() {
	c.store.Range(func(key, _ interface{}) bool {
		c.store.Delete(key)
		return true
	})
}

// StartCleanup runs a background goroutine that evicts expired entries every 5 minutes.
func (c *TTLCache) StartCleanup() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			now := time.Now()
			c.store.Range(func(key, v interface{}) bool {
				e := v.(*entry)
				if now.After(e.expires) {
					c.store.Delete(key)
				}
				return true
			})
		}
	}()
}
