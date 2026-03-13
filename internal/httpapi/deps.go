package httpapi

import (
	"context"
	"io"
	"time"

	"post-go/internal/core"
	redisx "post-go/internal/redis"
	"post-go/internal/s3"

	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
)

type redisStore interface {
	core.RedisStore
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd
	TTL(ctx context.Context, key string) *redis.DurationCmd
}

type fileObjectStore interface {
	UploadFile(ctx context.Context, filename string, size int64, contentType string, reader io.Reader, ttlSeconds int64) (string, error)
	GetObject(ctx context.Context, objectKey string) (*minio.Object, minio.ObjectInfo, error)
	DeleteObject(ctx context.Context, objectKey string) error
}

type handlerDependencies struct {
	getRedisStore  func(url string) (redisStore, error)
	newFileStore   func(conf s3.Config) (fileObjectStore, error)
	clearFileCache func(ctx context.Context, rdb redisStore, path string) error
	getFileCache   func(ctx context.Context, rdb redisStore, path string) (*core.FileCacheItem, error)
	setFileCache   func(ctx context.Context, rdb redisStore, path string, item *core.FileCacheItem) error
}

func defaultHandlerDependencies() handlerDependencies {
	return handlerDependencies{
		getRedisStore: func(url string) (redisStore, error) {
			return redisx.GetClient(url)
		},
		newFileStore: func(conf s3.Config) (fileObjectStore, error) {
			return s3.NewClient(conf)
		},
		clearFileCache: func(ctx context.Context, rdb redisStore, path string) error {
			return core.ClearFileCache(ctx, rdb, path)
		},
		getFileCache: func(ctx context.Context, rdb redisStore, path string) (*core.FileCacheItem, error) {
			return core.GetFileCache(ctx, rdb, path)
		},
		setFileCache: func(ctx context.Context, rdb redisStore, path string, item *core.FileCacheItem) error {
			return core.SetFileCache(ctx, rdb, path, item)
		},
	}
}
