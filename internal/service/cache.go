package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

type CacheService struct {
	redis *redis.Client
	ttl   time.Duration
}

func NewCacheServiceFromEnv(ttl time.Duration) *CacheService {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		slog.Info("Redis cache disabled: REDIS_URL is not set")
		return &CacheService{ttl: ttl}
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		slog.Error("Redis cache disabled: invalid REDIS_URL", "error", err)
		return &CacheService{ttl: ttl}
	}

	client := redis.NewClient(opts)
	slog.Info("Redis cache configured")
	return &CacheService{redis: client, ttl: ttl}
}

func (c *CacheService) Enabled() bool {
	return c != nil && c.redis != nil
}

func (c *CacheService) Get(ctx context.Context, key string, dest any) error {
	if !c.Enabled() {
		return redis.Nil
	}

	raw, err := c.redis.Get(ctx, key).Bytes()
	if err != nil {
		if err != redis.Nil {
			slog.Error("redis cache get failed", "key", key, "error", err)
		}
		return err
	}

	if err := json.Unmarshal(raw, dest); err != nil {
		slog.Error("redis cache unmarshal failed", "key", key, "error", err)
		return err
	}
	return nil
}

func (c *CacheService) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	if !c.Enabled() {
		return nil
	}
	if ttl <= 0 {
		ttl = c.ttl
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := c.redis.Set(ctx, key, raw, ttl).Err(); err != nil {
		slog.Error("redis cache set failed", "key", key, "error", err)
		return err
	}
	return nil
}

func (c *CacheService) GetOrSet(ctx context.Context, key string, fn func() (any, error)) (any, error) {
	if c == nil {
		return fn()
	}

	var cached any
	if err := c.Get(ctx, key, &cached); err == nil {
		return cached, nil
	}

	value, err := fn()
	if err != nil {
		return nil, err
	}
	_ = c.Set(ctx, key, value, c.ttl)
	return value, nil
}

func (c *CacheService) InvalidateProfileCache(ctx context.Context) {
	if !c.Enabled() {
		return
	}

	var cursor uint64
	for {
		keys, next, err := c.redis.Scan(ctx, cursor, "profiles:*", 500).Result()
		if err != nil {
			slog.Error("redis profile cache scan failed", "error", err)
			return
		}
		cursor = next

		if len(keys) > 0 {
			if err := c.redis.Del(ctx, keys...).Err(); err != nil {
				slog.Error("redis profile cache delete failed", "count", len(keys), "error", err)
			}
		}
		if cursor == 0 {
			return
		}
	}
}
