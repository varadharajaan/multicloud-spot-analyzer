// Package provider contains base implementations for cloud providers.
package provider

import (
	"sync"
	"time"

	"github.com/spot-analyzer/internal/config"
)

// CacheManager provides a global cache management system
// with configurable TTL and manual refresh capability
type CacheManager struct {
	mu          sync.RWMutex
	items       map[string]cacheEntry
	defaultTTL  time.Duration
	stats       CacheStats
	lastRefresh time.Time
}

type cacheEntry struct {
	value      interface{}
	expiration time.Time
	key        string
	ttl        time.Duration
	createdAt  time.Time
}

// CacheStats holds cache statistics
type CacheStats struct {
	Hits       int64     `json:"hits"`
	Misses     int64     `json:"misses"`
	Items      int       `json:"items"`
	LastClear  time.Time `json:"last_clear"`
	OldestItem time.Time `json:"oldest_item"`
}

var (
	globalCacheManager *CacheManager
	cacheOnce          sync.Once
)

// GetCacheManager returns the global cache manager singleton
func GetCacheManager() *CacheManager {
	cacheOnce.Do(func() {
		cfg := config.Get()
		globalCacheManager = &CacheManager{
			items:      make(map[string]cacheEntry),
			defaultTTL: cfg.Cache.TTL,
		}
		go globalCacheManager.cleanup()
	})
	return globalCacheManager
}

// Get retrieves a value from the cache
func (cm *CacheManager) Get(key string) (interface{}, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	entry, exists := cm.items[key]
	if !exists {
		cm.stats.Misses++
		return nil, false
	}

	if time.Now().After(entry.expiration) {
		cm.stats.Misses++
		return nil, false
	}

	cm.stats.Hits++
	return entry.value, true
}

// Set stores a value in the cache with the specified TTL
func (cm *CacheManager) Set(key string, value interface{}, ttl time.Duration) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	cm.items[key] = cacheEntry{
		value:      value,
		expiration: now.Add(ttl),
		key:        key,
		ttl:        ttl,
		createdAt:  now,
	}
	cm.stats.Items = len(cm.items)
}

// SetWithDefaultTTL stores a value with the default TTL
func (cm *CacheManager) SetWithDefaultTTL(key string, value interface{}) {
	cm.Set(key, value, cm.defaultTTL)
}

// Delete removes a specific key from the cache
func (cm *CacheManager) Delete(key string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.items, key)
	cm.stats.Items = len(cm.items)
}

// DeletePrefix removes all keys with a given prefix
func (cm *CacheManager) DeletePrefix(prefix string) int {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	count := 0
	for key := range cm.items {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(cm.items, key)
			count++
		}
	}
	cm.stats.Items = len(cm.items)
	return count
}

// Clear removes all items from the cache
func (cm *CacheManager) Clear() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.items = make(map[string]cacheEntry)
	cm.stats.Items = 0
	cm.stats.LastClear = time.Now()
	cm.lastRefresh = time.Now()
}

// Refresh clears all data and resets for fresh fetches
func (cm *CacheManager) Refresh() {
	cm.Clear()
}

// GetStats returns cache statistics
func (cm *CacheManager) GetStats() CacheStats {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	stats := cm.stats
	stats.Items = len(cm.items)

	// Find oldest item
	var oldest time.Time
	for _, entry := range cm.items {
		if oldest.IsZero() || entry.createdAt.Before(oldest) {
			oldest = entry.createdAt
		}
	}
	stats.OldestItem = oldest

	return stats
}

// GetTTL returns the default TTL
func (cm *CacheManager) GetTTL() time.Duration {
	return cm.defaultTTL
}

// SetDefaultTTL sets the default TTL for new entries
func (cm *CacheManager) SetDefaultTTL(ttl time.Duration) {
	cm.defaultTTL = ttl
}

// GetLastRefresh returns when cache was last cleared
func (cm *CacheManager) GetLastRefresh() time.Time {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.lastRefresh
}

// Keys returns all cache keys (for debugging)
func (cm *CacheManager) Keys() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	keys := make([]string, 0, len(cm.items))
	for key := range cm.items {
		keys = append(keys, key)
	}
	return keys
}

// cleanup periodically removes expired items
func (cm *CacheManager) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		cm.mu.Lock()
		now := time.Now()
		for key, entry := range cm.items {
			if now.After(entry.expiration) {
				delete(cm.items, key)
			}
		}
		cm.stats.Items = len(cm.items)
		cm.mu.Unlock()
	}
}
