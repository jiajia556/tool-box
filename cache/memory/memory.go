package memory

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/jiajia556/tool-box/cache"
)

type item struct {
	Value      json.RawMessage
	Expiration time.Time
}

type MemoryCache struct {
	mu    sync.RWMutex
	items map[string]*item
	stats cache.Stats
}

// NewMemoryCache create new memory cache
func NewMemoryCache() cache.Cache {
	return &MemoryCache{
		items: make(map[string]*item),
	}
}

func (m *MemoryCache) Get(key string) (any, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, ok := m.items[key]
	if !ok {
		m.stats.Misses++
		return nil, false
	}

	// 检查是否过期
	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		m.stats.Misses++
		return nil, false
	}

	var v any
	if err := json.Unmarshal(item.Value, &v); err != nil {
		m.stats.Misses++
		return nil, false
	}

	m.stats.Hits++
	return v, true
}

func (m *MemoryCache) Set(key string, value any, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	b, err := json.Marshal(value)
	if err != nil {
		return
	}

	var expiration time.Time
	if ttl > 0 {
		expiration = time.Now().Add(ttl)
	}

	m.items[key] = &item{
		Value:      b,
		Expiration: expiration,
	}

	m.stats.Sets++
}

func (m *MemoryCache) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.items, key)
	m.stats.Deletes++
}

func (m *MemoryCache) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.items = make(map[string]*item)
}

func (m *MemoryCache) Exists(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, ok := m.items[key]
	if !ok {
		return false
	}

	// 检查是否过期
	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		return false
	}

	return true
}

func (m *MemoryCache) TTL(key string) (time.Duration, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	item, ok := m.items[key]
	if !ok {
		return 0, false
	}

	if item.Expiration.IsZero() {
		return 0, false
	}

	ttl := time.Until(item.Expiration)
	if ttl <= 0 {
		return 0, false
	}

	return ttl, true
}

func (m *MemoryCache) Stats() cache.Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.stats
}

func (m *MemoryCache) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.items = make(map[string]*item)
	return nil
}

func (m *MemoryCache) Start(config any) error {
	// 内存缓存不需要配置
	return nil
}

func init() {
	cache.Register("memory", NewMemoryCache)
}
