package core

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore captures the Redis operations used by file cache helpers.
type RedisStore interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	SetEx(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Unlink(ctx context.Context, keys ...string) *redis.IntCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}
