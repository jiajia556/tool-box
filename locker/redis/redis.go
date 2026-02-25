package redis

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/jiajia556/tool-box/locker"
)

var (
	globalClient *redis.Client
	clientMu     sync.RWMutex
)

// Options Redis 配置选项
type Options struct {
	Addr     string        `json:"addr"`
	Username string        `json:"username"`
	Password string        `json:"password"`
	DB       int           `json:"db"`
	Timeout  time.Duration `json:"timeout"`
}

// RedisManager Redis 分布式锁管理器
type RedisManager struct {
	mu    sync.RWMutex
	locks map[string]*redisLocker
}

// redisLocker Redis 锁实现
type redisLocker struct {
	manager         *RedisManager
	key             string
	token           string
	config          locker.Config
	ctx             context.Context
	cancel          context.CancelFunc
	refreshTicker   *time.Ticker
	refreshStopChan chan struct{}
	mu              sync.Mutex
	locked          bool
}

// NewRedisManager 创建 Redis 锁管理器，同时初始化全局 Redis 客户端
func NewRedisManager(config any) (locker.Manager, error) {
	opts := Options{
		Addr:    "localhost:6379",
		DB:      0,
		Timeout: 5 * time.Second,
	}

	// 如果提供了配置，则使用提供的配置
	if config != nil {
		if redisOpts, ok := config.(Options); ok {
			opts = redisOpts
		} else {
			return nil, fmt.Errorf("redis: invalid config type, expect redis.Options")
		}
	}

	// 初始化 Redis 客户端
	if err := initRedisClient(opts); err != nil {
		return nil, err
	}

	return &RedisManager{
		locks: make(map[string]*redisLocker),
	}, nil
}

// initRedisClient 初始化全局 Redis 客户端
func initRedisClient(opts Options) error {
	clientMu.Lock()
	defer clientMu.Unlock()

	// 如果已经初始化，先关闭旧的连接
	if globalClient != nil {
		_ = globalClient.Close()
	}

	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Second
	}

	client := redis.NewClient(&redis.Options{
		Addr:         opts.Addr,
		Username:     opts.Username,
		Password:     opts.Password,
		DB:           opts.DB,
		DialTimeout:  opts.Timeout,
		ReadTimeout:  opts.Timeout,
		WriteTimeout: opts.Timeout,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}

	globalClient = client
	return nil
}

// New 创建新的锁
func (rm *RedisManager) New(key string, opts ...locker.Option) locker.Locker {
	config := locker.DefaultConfig()
	for _, opt := range opts {
		opt(&config)
	}

	token := uuid.New().String()

	ctx, cancel := context.WithCancel(context.Background())

	l := &redisLocker{
		manager:         rm,
		key:             key,
		token:           token,
		config:          config,
		ctx:             ctx,
		cancel:          cancel,
		refreshStopChan: make(chan struct{}, 1),
		locked:          false,
	}

	rm.mu.Lock()
	rm.locks[token] = l
	rm.mu.Unlock()

	return l
}

// TryLock 尝试获取锁
func (rl *redisLocker) TryLock(ctx context.Context) (bool, error) {
	clientMu.RLock()
	client := globalClient
	clientMu.RUnlock()

	if client == nil {
		return false, fmt.Errorf("redis client not initialized")
	}

	rl.mu.Lock()
	if rl.locked {
		rl.mu.Unlock()
		return false, locker.ErrLockFailed
	}
	rl.mu.Unlock()

	// 使用 Redis SET NX 命令原子性地设置锁
	ok, err := client.SetNX(ctx, rl.key, rl.token, rl.config.TTL).Result()
	if err != nil {
		return false, err
	}

	if ok {
		rl.mu.Lock()
		rl.locked = true
		rl.mu.Unlock()

		// 启动自动续期
		if rl.config.RefreshInterval > 0 {
			rl.startRefresh()
		}

		return true, nil
	}

	return false, nil
}

// Lock 获取锁（阻塞）
func (rl *redisLocker) Lock(ctx context.Context) error {
	deadline := time.Now().Add(rl.config.Timeout)

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
		acquired, err := rl.TryLock(ctx)
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
		case <-time.After(rl.config.PollInterval):
		}
	}
}

// Unlock 释放锁
func (rl *redisLocker) Unlock(ctx context.Context) error {
	clientMu.RLock()
	client := globalClient
	clientMu.RUnlock()

	if client == nil {
		return fmt.Errorf("redis client not initialized")
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if !rl.locked {
		return locker.ErrLockNotHeld
	}

	// 停止自动续期
	rl.stopRefresh()

	// 检查锁是否仍然被当前 holder 持有
	val, err := client.Get(ctx, rl.key).Result()
	if err == redis.Nil {
		rl.locked = false
		return nil
	}
	if err != nil {
		return err
	}

	// 验证 token 匹配
	if val != rl.token {
		return locker.ErrLockNotHeld
	}

	// 使用 Lua 脚本确保原子性删除
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`)

	result, err := script.Run(ctx, client, []string{rl.key}, rl.token).Result()
	if err != nil {
		return err
	}

	if result.(int64) > 0 {
		rl.locked = false
		return nil
	}

	return locker.ErrLockNotHeld
}

// TTL 获取锁的剩余时间
func (rl *redisLocker) TTL(ctx context.Context) (time.Duration, error) {
	clientMu.RLock()
	client := globalClient
	clientMu.RUnlock()

	if client == nil {
		return 0, fmt.Errorf("redis client not initialized")
	}

	rl.mu.Lock()
	if !rl.locked {
		rl.mu.Unlock()
		return 0, locker.ErrLockNotHeld
	}
	rl.mu.Unlock()

	ttl, err := client.TTL(ctx, rl.key).Result()
	if err != nil {
		return 0, err
	}

	if ttl < 0 {
		return 0, locker.ErrLockNotHeld
	}

	return ttl, nil
}

// Refresh 刷新锁的过期时间
func (rl *redisLocker) Refresh(ctx context.Context, ttl time.Duration) error {
	clientMu.RLock()
	client := globalClient
	clientMu.RUnlock()

	if client == nil {
		return fmt.Errorf("redis client not initialized")
	}

	rl.mu.Lock()
	if !rl.locked {
		rl.mu.Unlock()
		return locker.ErrLockNotHeld
	}
	rl.mu.Unlock()

	// 使用 Lua 脚本确保原子性续期
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("PEXPIRE", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)

	result, err := script.Run(ctx, client, []string{rl.key}, rl.token, int64(ttl/time.Millisecond)).Result()
	if err != nil {
		return err
	}

	if result.(int64) > 0 {
		return nil
	}

	return locker.ErrLockNotHeld
}

// Token 获取锁的令牌
func (rl *redisLocker) Token() string {
	return rl.token
}

// Key 获取锁的 key
func (rl *redisLocker) Key() string {
	return rl.key
}

// Close 关闭锁
func (rl *redisLocker) Close() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if rl.locked {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = rl.Unlock(ctx)
		cancel()
	}

	rl.cancel()
	rl.stopRefresh()

	rl.manager.mu.Lock()
	delete(rl.manager.locks, rl.token)
	rl.manager.mu.Unlock()

	return nil
}

// startRefresh 启动自动续期
func (rl *redisLocker) startRefresh() {
	rl.refreshTicker = time.NewTicker(rl.config.RefreshInterval)

	go func() {
		for {
			select {
			case <-rl.ctx.Done():
				rl.refreshTicker.Stop()
				return
			case <-rl.refreshStopChan:
				rl.refreshTicker.Stop()
				return
			case <-rl.refreshTicker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if err := rl.Refresh(ctx, rl.config.TTL); err != nil {
					fmt.Printf("locker refresh failed: %v\n", err)
				}
				cancel()
			}
		}
	}()
}

// stopRefresh 停止自动续期
func (rl *redisLocker) stopRefresh() {
	if rl.refreshTicker != nil {
		rl.refreshTicker.Stop()
		rl.refreshTicker = nil
	}
	select {
	case rl.refreshStopChan <- struct{}{}:
	default:
	}
}

// Close 关闭锁管理器
func (rm *RedisManager) Close() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	for _, l := range rm.locks {
		_ = l.Close()
	}
	rm.locks = make(map[string]*redisLocker)

	clientMu.Lock()
	if globalClient != nil {
		_ = globalClient.Close()
		globalClient = nil
	}
	clientMu.Unlock()

	return nil
}

func init() {
	locker.Register("redis", NewRedisManager)
}
