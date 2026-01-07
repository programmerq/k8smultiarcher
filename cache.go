package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/bluele/gcache"
	"github.com/redis/go-redis/v9"
)

const (
	cacheSizeDefault = 100000
	redisAddrDefault = "localhost:6379"
)

type Cache interface {
	Get(key string) (bool, bool)
	Set(key string, value bool, ttl time.Duration)
}

type InMemoryCache struct {
	cache gcache.Cache
}

func NewInMemoryCache(cacheSize int) *InMemoryCache {
	gc := gcache.New(cacheSize).ARC().Build()
	return &InMemoryCache{gc}
}

func (c InMemoryCache) Get(key string) (bool, bool) {
	val, err := c.cache.Get(key)
	if err != nil {
		return false, false
	}
	boolVal, ok := val.(bool)
	if !ok {
		slog.Error("found non boolean cache value")
		return false, false
	}
	return boolVal, true
}

func (c *InMemoryCache) Set(key string, value bool, ttl time.Duration) {
	var err error
	if ttl > 0 {
		err = c.cache.SetWithExpire(key, value, ttl)
	} else {
		err = c.cache.Set(key, value)
	}
	if err != nil {
		slog.Error("failed to set key on InMemoryCache", "error", err)
	}
}

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(redisAddr string) *RedisCache {
	return &RedisCache{
		redis.NewClient(&redis.Options{
			Addr: redisAddr,
		}),
	}
}

func (c RedisCache) Get(key string) (bool, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	val, err := c.client.Get(ctx, key).Bool()
	if err != nil {
		return false, false
	}
	return val, true
}

func (c *RedisCache) Set(key string, value bool, ttl time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := c.client.Set(ctx, key, value, ttl).Err()
	if err != nil {
		slog.Error("failed to set key on RedisCache", "error", err)
	}
}
