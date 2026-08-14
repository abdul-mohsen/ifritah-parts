package service

import (
	"sync"
	"time"

	"parts-engine/internal/model"
)

type vinCacheEntry struct {
	result    *model.VINDecodeResult
	expiresAt time.Time
}

// VINCache is a simple in-memory TTL cache for decoded VINs.
type VINCache struct {
	mu      sync.RWMutex
	entries map[string]*vinCacheEntry
	ttl     time.Duration
}

func NewVINCache(ttl time.Duration) *VINCache {
	c := &VINCache{
		entries: make(map[string]*vinCacheEntry),
		ttl:     ttl,
	}
	go c.cleanup()
	return c
}

func (c *VINCache) Get(vin string) (*model.VINDecodeResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[vin]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.result, true
}

func (c *VINCache) Set(vin string, result *model.VINDecodeResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[vin] = &vinCacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *VINCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for k, v := range c.entries {
			if now.After(v.expiresAt) {
				delete(c.entries, k)
			}
		}
		c.mu.Unlock()
	}
}
