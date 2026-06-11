package core

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

const fileCacheTTL = time.Hour
const maxJSONSafeInteger = int64(1<<53 - 1)

func fileCacheKey(path string) string {
	return "cache:file:" + path
}

// FileCacheItem stores cached file body and metadata.
type FileCacheItem struct {
	Buffer        []byte
	ContentType   string
	ContentLength int64
}

type fileCacheMetadata struct {
	ContentType   string `json:"ct"`
	ContentLength int64  `json:"cl"`
}

type fileCacheMetadataDecode struct {
	ContentType   *string `json:"ct"`
	ContentLength *int64  `json:"cl"`
}

// GetFileCache reads cached file if exists.
func GetFileCache(ctx context.Context, rdb RedisStore, path string) (*FileCacheItem, error) {
	stored, err := rdb.Get(ctx, fileCacheKey(path)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	payload := []byte(stored)
	if len(payload) < 4 {
		return nil, nil
	}
	metaLength := binary.BigEndian.Uint32(payload[:4])
	if metaLength > uint32(len(payload)-4) {
		return nil, nil
	}
	bodyStart := 4 + int(metaLength)
	var meta fileCacheMetadataDecode
	if err := json.Unmarshal(payload[4:bodyStart], &meta); err != nil {
		return nil, nil
	}
	body := payload[bodyStart:]
	if meta.ContentType == nil || meta.ContentLength == nil || *meta.ContentLength < 0 || *meta.ContentLength > maxJSONSafeInteger {
		return nil, nil
	}
	return &FileCacheItem{
		Buffer:        body,
		ContentType:   *meta.ContentType,
		ContentLength: *meta.ContentLength,
	}, nil
}

// SetFileCache writes cached file to Redis.
func SetFileCache(ctx context.Context, rdb RedisStore, path string, item *FileCacheItem) error {
	contentType := item.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	meta := fileCacheMetadata{
		ContentType:   contentType,
		ContentLength: item.ContentLength,
	}
	metaBytes, _ := json.Marshal(&meta)
	payload := make([]byte, 4+len(metaBytes)+len(item.Buffer))
	binary.BigEndian.PutUint32(payload[:4], uint32(len(metaBytes)))
	copy(payload[4:], metaBytes)
	copy(payload[4+len(metaBytes):], item.Buffer)
	return rdb.SetEx(ctx, fileCacheKey(path), payload, fileCacheTTL).Err()
}

// ClearFileCache removes cache keys.
func ClearFileCache(ctx context.Context, rdb RedisStore, path string) error {
	if err := rdb.Unlink(ctx, fileCacheKey(path)).Err(); err == nil {
		return nil
	}
	return rdb.Del(ctx, fileCacheKey(path)).Err()
}
