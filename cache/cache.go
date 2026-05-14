package cache

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type Cache interface {
	Get(key string) (any, error)
	Set(key string, value any, ttl time.Duration)
	Delete(key string)
	Clear()
	TTL(key string) (time.Duration, bool)
	Exists(key string) bool
	Stats() Stats
	Close() error
	Start(config any) error
}

type Instance func() Cache

var (
	adaptersMu sync.RWMutex
	adapters   = make(map[string]Instance)
)

const (
	AdapterMemory = "memory"
	AdapterRedis  = "redis"
	AdapterFile   = "file"
)

var (
	global Cache
	once   sync.Once
)

var (
	ErrNoGlobal     = errors.New("cache: global instance is nil")
	ErrNotFound     = errors.New("cache: not found")
	ErrTypeMismatch = errors.New("cache: type mismatch")
	ErrDecode       = errors.New("cache: decode failed")
)

func Init(adapterName string, config ...any) (err error) {
	instanceFunc, ok := adapters[adapterName]
	if !ok {
		err = fmt.Errorf("cache: unknown adapter name %q (forgot to import?)", adapterName)
		return
	}
	once.Do(func() {
		global = instanceFunc()
		err = global.Start(config[0])
	})
	if err != nil {
		global = nil
	}
	return
}

func SetGlobal(cache Cache) {
	global = cache
}

func Register(name string, adapter Instance) {
	adaptersMu.Lock()
	defer adaptersMu.Unlock()
	if adapter == nil {
		panic("cache: Register adapter is nil")
	}
	if _, ok := adapters[name]; ok {
		panic("cache: Register called twice for adapter " + name)
	}
	adapters[name] = adapter
}

func Get[T any](key string) (T, error) {
	var zero T
	if global == nil {
		return zero, ErrNoGlobal
	}

	v, err := global.Get(key)
	if err != nil {
		return zero, err
	}

	tv, ok := v.(T)
	if !ok {
		return zero, fmt.Errorf("%w: value type %T", ErrTypeMismatch, v)
	}

	return tv, nil
}

func Set[T any](key string, value T, ttl time.Duration) {
	if global == nil {
		return
	}
	global.Set(key, value, ttl)
}

func Delete(key string) {
	if global == nil {
		return
	}
	global.Delete(key)
}

func Exists(key string) bool {
	if global == nil {
		return false
	}
	return global.Exists(key)
}

func TTL(key string) (time.Duration, bool) {
	if global == nil {
		return 0, false
	}
	return global.TTL(key)
}

func GetStats() Stats {
	if global == nil {
		return Stats{}
	}
	return global.Stats()
}

func Close() error {
	if global == nil {
		return nil
	}
	return global.Close()
}
