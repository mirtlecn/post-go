package core

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore captures the Redis operations used by file cache helpers.
type RedisStore interface {
	MGet(ctx context.Context, keys ...string) *redis.SliceCmd
	SetEx(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Unlink(ctx context.Context, keys ...string) *redis.IntCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	TxPipeline() redis.Pipeliner
}
