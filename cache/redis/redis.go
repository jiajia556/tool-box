package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"github.com/jiajia556/tool-box/cache"
)

type RedisCache struct {
	client *redis.Client
	opts   Options
	stats  cache.Stats
	sf     singleflight.Group
	ctx    context.Context
}

// NewRedisCache create new redis cache with default collection name.
func NewRedisCache() cache.Cache {
	return &RedisCache{}
}

func (r *RedisCache) key(k string) string {
	if r.opts.Prefix == "" {
		return k
	}
	return r.opts.Prefix + ":" + k
}

// ---------------- basic ----------------

func (r *RedisCache) Get(key string) (any, bool) {
	b, err := r.client.Get(r.ctx, r.key(key)).Bytes()
	if err == redis.Nil {
		r.stats.Misses++
		return nil, false
	}
	if err != nil {
		r.stats.Misses++
		return nil, false
	}

	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		r.stats.Misses++
		return nil, false
	}

	r.stats.Hits++
	return v, true
}

func (r *RedisCache) Set(key string, value any, ttl time.Duration) {
	if ttl <= 0 {
		ttl = r.opts.DefaultTTL
	}

	b, err := json.Marshal(value)
	if err != nil {
		return
	}

	if ttl > 0 {
		_ = r.client.Set(r.ctx, r.key(key), b, ttl).Err()
	} else {
		_ = r.client.Set(r.ctx, r.key(key), b, 0).Err()
	}

	r.stats.Sets++
}

func (r *RedisCache) Delete(key string) {
	_ = r.client.Del(r.ctx, r.key(key)).Err()
	r.stats.Deletes++
}

func (r *RedisCache) Clear() {
	if r.opts.Prefix == "" {
		_ = r.client.FlushDB(r.ctx).Err()
		return
	}

	iter := r.client.Scan(r.ctx, 0, r.opts.Prefix+":*", 0).Iterator()
	for iter.Next(r.ctx) {
		_ = r.client.Del(r.ctx, iter.Val()).Err()
	}
}

func (r *RedisCache) Exists(key string) bool {
	n, err := r.client.Exists(r.ctx, r.key(key)).Result()
	return err == nil && n > 0
}

func (r *RedisCache) TTL(key string) (time.Duration, bool) {
	d, err := r.client.TTL(r.ctx, r.key(key)).Result()
	if err != nil || d <= 0 {
		return 0, false
	}
	return d, true
}

func (r *RedisCache) Stats() cache.Stats {
	return r.stats
}

func (r *RedisCache) Close() error {
	return r.client.Close()
}

func (r *RedisCache) Start(config any) error {
	opts, ok := config.(Options)
	if !ok {
		return fmt.Errorf("redis cache: invalid config")
	}
	r.opts = opts

	rdb := redis.NewClient(&redis.Options{
		Addr:     opts.Addr,
		Username: opts.Username,
		Password: opts.Password,
		DB:       opts.DB,
	})

	r.client = rdb
	r.ctx = context.Background()

	return nil
}

func init() {
	cache.Register("redis", NewRedisCache)
}
