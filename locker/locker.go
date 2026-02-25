package locker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrLockFailed    = errors.New("failed to acquire lock")
	ErrLockNotHeld   = errors.New("lock is not held by current holder")
	ErrWaitTimeout   = errors.New("wait for lock timeout")
	ErrInvalidConfig = errors.New("invalid lock config")
)

// Locker 分布式锁接口
type Locker interface {
	// 尝试获取锁（非阻塞）
	TryLock(ctx context.Context) (bool, error)

	// 获取锁（阻塞，直到获得锁或超时）
	Lock(ctx context.Context) error

	// 释放锁
	Unlock(ctx context.Context) error

	// 获取锁的剩余 TTL
	TTL(ctx context.Context) (time.Duration, error)

	// 刷新锁的过期时间
	Refresh(ctx context.Context, ttl time.Duration) error

	// 获取锁的唯一标识符（token）
	Token() string

	// 获取锁的 key
	Key() string

	// 关闭锁（释放资源）
	Close() error
}

// Manager 锁管理器接口
type Manager interface {
	// 创建新的锁
	New(key string, opts ...Option) Locker

	// 关闭锁管理器
	Close() error
}

// Config 锁配置
type Config struct {
	// 锁的 TTL（生存时间）
	TTL time.Duration

	// 尝试获取锁的超时时间
	Timeout time.Duration

	// 轮询间隔
	PollInterval time.Duration

	// 自动续期间隔（小于等于0表示不自动续期）
	RefreshInterval time.Duration

	// 是否在释放时自动关闭
	AutoClose bool
}

// Option 选项函数
type Option func(*Config)

// WithTTL 设置 TTL
func WithTTL(ttl time.Duration) Option {
	return func(c *Config) {
		c.TTL = ttl
	}
}

// WithTimeout 设置获取锁的超时时间
func WithTimeout(timeout time.Duration) Option {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithPollInterval 设置轮询间隔
func WithPollInterval(interval time.Duration) Option {
	return func(c *Config) {
		c.PollInterval = interval
	}
}

// WithRefreshInterval 设置自动续期间隔
func WithRefreshInterval(interval time.Duration) Option {
	return func(c *Config) {
		c.RefreshInterval = interval
	}
}

// WithAutoClose 设置是否自动关闭
func WithAutoClose(autoClose bool) Option {
	return func(c *Config) {
		c.AutoClose = autoClose
	}
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
	return Config{
		TTL:             30 * time.Second,
		Timeout:         5 * time.Second,
		PollInterval:    100 * time.Millisecond,
		RefreshInterval: 0,
		AutoClose:       true,
	}
}

// Instance 适配器工厂函数
type Instance func(config any) (Manager, error)

var (
	adaptersMu sync.RWMutex
	adapters   = make(map[string]Instance)
)

const (
	AdapterMemory = "memory"
	AdapterRedis  = "redis"
)

var (
	globalManager Manager
	once          sync.Once
)

// Register 注册锁适配器
func Register(name string, adapter Instance) {
	adaptersMu.Lock()
	defer adaptersMu.Unlock()

	if adapter == nil {
		panic("locker: Register adapter is nil")
	}
	if _, ok := adapters[name]; ok {
		panic("locker: Register called twice for adapter " + name)
	}
	adapters[name] = adapter
}

// Init 初始化全局锁管理器
// 参数 config 是可选的，不同的适配器接受不同的配置类型：
// - "memory": 无需配置
// - "redis": 接受 redis.Options 结构体
func Init(adapterName string, config ...any) (err error) {
	adaptersMu.RLock()
	instanceFunc, ok := adapters[adapterName]
	adaptersMu.RUnlock()

	if !ok {
		return fmt.Errorf("locker: unknown adapter name %q (forgot to import?)", adapterName)
	}

	once.Do(func() {
		var cfg any
		if len(config) > 0 {
			cfg = config[0]
		}

		globalManager, err = instanceFunc(cfg)
	})

	return
}

// New 创建新的锁（使用全局锁管理器）
func New(key string, opts ...Option) Locker {
	if globalManager == nil {
		return nil
	}
	return globalManager.New(key, opts...)
}

// TryLock 尝试获取锁
func TryLock(ctx context.Context, key string, opts ...Option) (bool, error) {
	lock := New(key, opts...)
	if lock == nil {
		return false, fmt.Errorf("global manager not initialized")
	}
	defer lock.Close()
	return lock.TryLock(ctx)
}

// Lock 获取锁（阻塞）
func Lock(ctx context.Context, key string, opts ...Option) (Locker, error) {
	lock := New(key, opts...)
	if lock == nil {
		return nil, fmt.Errorf("global manager not initialized")
	}
	if err := lock.Lock(ctx); err != nil {
		lock.Close()
		return nil, err
	}
	return lock, nil
}

// Close 关闭全局锁管理器
func Close() error {
	if globalManager == nil {
		return nil
	}
	return globalManager.Close()
}
