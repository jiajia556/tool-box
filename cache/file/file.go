package file

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jiajia556/tool-box/cache"
)

type fileItem struct {
	Value      json.RawMessage `json:"value"`
	Expiration time.Time       `json:"expiration"`
}

type FileCache struct {
	dir   string
	mu    sync.RWMutex
	stats cache.Stats
}

type Options struct {
	Dir string `json:"dir"`
}

// NewFileCache create new file cache
func NewFileCache() cache.Cache {
	return &FileCache{}
}

func (f *FileCache) getFilePath(key string) string {
	return filepath.Join(f.dir, key+".cache.json")
}

// 确保目录存在
func (f *FileCache) ensureDir() error {
	return os.MkdirAll(f.dir, 0755)
}

func (f *FileCache) Get(key string) (any, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	filePath := f.getFilePath(key)
	data, err := os.ReadFile(filePath)
	if err != nil {
		f.stats.Misses++
		return nil, false
	}

	var item fileItem
	if err := json.Unmarshal(data, &item); err != nil {
		f.stats.Misses++
		return nil, false
	}

	// 检查是否过期
	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		f.stats.Misses++
		// 异步删除过期文件
		go func() {
			_ = os.Remove(filePath)
		}()
		return nil, false
	}

	var v any
	if err := json.Unmarshal(item.Value, &v); err != nil {
		f.stats.Misses++
		return nil, false
	}

	f.stats.Hits++
	return v, true
}

func (f *FileCache) Set(key string, value any, ttl time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()

	b, err := json.Marshal(value)
	if err != nil {
		return
	}

	var expiration time.Time
	if ttl > 0 {
		expiration = time.Now().Add(ttl)
	}

	item := fileItem{
		Value:      b,
		Expiration: expiration,
	}

	data, err := json.Marshal(item)
	if err != nil {
		return
	}

	filePath := f.getFilePath(key)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return
	}

	f.stats.Sets++
}

func (f *FileCache) Delete(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	filePath := f.getFilePath(key)
	_ = os.Remove(filePath)
	f.stats.Deletes++
}

func (f *FileCache) Clear() {
	f.mu.Lock()
	defer f.mu.Unlock()

	entries, err := os.ReadDir(f.dir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			_ = os.Remove(filepath.Join(f.dir, entry.Name()))
		}
	}
}

func (f *FileCache) Exists(key string) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	filePath := f.getFilePath(key)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}

	var item fileItem
	if err := json.Unmarshal(data, &item); err != nil {
		return false
	}

	// 检查是否过期
	if !item.Expiration.IsZero() && time.Now().After(item.Expiration) {
		return false
	}

	return true
}

func (f *FileCache) TTL(key string) (time.Duration, bool) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	filePath := f.getFilePath(key)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, false
	}

	var item fileItem
	if err := json.Unmarshal(data, &item); err != nil {
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

func (f *FileCache) Stats() cache.Stats {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.stats
}

func (f *FileCache) Close() error {
	// 可选：关闭时清空缓存或执行清理
	return nil
}

func (f *FileCache) Start(config any) error {
	opts, ok := config.(Options)
	if !ok {
		return fmt.Errorf("file cache: invalid config")
	}

	if opts.Dir == "" {
		opts.Dir = "./cache"
	}

	f.dir = opts.Dir

	if err := f.ensureDir(); err != nil {
		return fmt.Errorf("file cache: failed to create cache directory: %w", err)
	}

	return nil
}

func init() {
	cache.Register("file", NewFileCache)
}
