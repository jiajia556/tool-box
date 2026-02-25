package memory

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/jiajia556/tool-box/locker"
)

// MemoryManager 内存锁管理器（单机用）
type MemoryManager struct {
	mu    sync.RWMutex
	locks map[string]*memoryLocker
}

// memoryLocker 内存锁实现
type memoryLocker struct {
	manager    *MemoryManager
	key        string
	token      string
	config     locker.Config
	expireTime time.Time
	mu         sync.Mutex
	locked     bool
}

// NewMemoryManager 创建内存锁管理器
func NewMemoryManager(config any) (locker.Manager, error) {
	return &MemoryManager{
		locks: make(map[string]*memoryLocker),
	}, nil
}

// New 创建新的锁
func (mm *MemoryManager) New(key string, opts ...locker.Option) locker.Locker {
	config := locker.DefaultConfig()
	for _, opt := range opts {
		opt(&config)
	}

	token := uuid.New().String()

	return &memoryLocker{
		manager: mm,
		key:     key,
		token:   token,
		config:  config,
		locked:  false,
	}
}

// TryLock 尝试获取锁
func (ml *memoryLocker) TryLock(ctx context.Context) (bool, error) {
	ml.manager.mu.Lock()
	defer ml.manager.mu.Unlock()

	// 检查锁是否存在且未过期
	if existingLock, ok := ml.manager.locks[ml.key]; ok {
		if time.Now().Before(existingLock.expireTime) {
			return false, nil
		}
		// 锁已过期，删除它
		delete(ml.manager.locks, ml.key)
	}

	// 获取锁
	ml.expireTime = time.Now().Add(ml.config.TTL)
	ml.manager.locks[ml.key] = ml
	ml.locked = true

	return true, nil
}

// Lock 获取锁（阻塞）
func (ml *memoryLocker) Lock(ctx context.Context) error {
	deadline := time.Now().Add(ml.config.Timeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 检查是否超时
		if time.Now().After(deadline) {
			return locker.ErrWaitTimeout
		}

		// 尝试获取锁
		acquired, err := ml.TryLock(ctx)
		if err != nil {
			return err
		}

		if acquired {
			return nil
		}

		// 等待后重试
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(ml.config.PollInterval):
		}
	}
}

// Unlock 释放锁
func (ml *memoryLocker) Unlock(ctx context.Context) error {
	ml.manager.mu.Lock()
	defer ml.manager.mu.Unlock()

	if !ml.locked {
		return locker.ErrLockNotHeld
	}

	existingLock, ok := ml.manager.locks[ml.key]
	if !ok || existingLock.token != ml.token {
		return locker.ErrLockNotHeld
	}

	delete(ml.manager.locks, ml.key)
	ml.locked = false

	return nil
}

// TTL 获取锁的剩余时间
func (ml *memoryLocker) TTL(ctx context.Context) (time.Duration, error) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	if !ml.locked {
		return 0, locker.ErrLockNotHeld
	}

	ttl := time.Until(ml.expireTime)
	if ttl < 0 {
		return 0, locker.ErrLockNotHeld
	}

	return ttl, nil
}

// Refresh 刷新锁的过期时间
func (ml *memoryLocker) Refresh(ctx context.Context, ttl time.Duration) error {
	ml.manager.mu.Lock()
	defer ml.manager.mu.Unlock()

	existingLock, ok := ml.manager.locks[ml.key]
	if !ok || existingLock.token != ml.token {
		return locker.ErrLockNotHeld
	}

	ml.expireTime = time.Now().Add(ttl)
	return nil
}

// Token 获取锁的令牌
func (ml *memoryLocker) Token() string {
	return ml.token
}

// Key 获取锁的 key
func (ml *memoryLocker) Key() string {
	return ml.key
}

// Close 关闭锁
func (ml *memoryLocker) Close() error {
	ml.manager.mu.Lock()
	defer ml.manager.mu.Unlock()

	if ml.locked {
		delete(ml.manager.locks, ml.key)
		ml.locked = false
	}

	return nil
}

// Close 关闭锁管理器
func (mm *MemoryManager) Close() error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.locks = make(map[string]*memoryLocker)
	return nil
}

func init() {
	locker.Register("memory", NewMemoryManager)
}
