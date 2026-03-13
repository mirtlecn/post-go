package core

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"
)

const fileCacheTTL = time.Hour

func fileCacheKey(path string) string {
	return "cache:file:" + path
}

func metaCacheKey(path string) string {
	return "cache:filemeta:" + path
}

// FileCacheItem stores cached file body and metadata.
type FileCacheItem struct {
	Buffer        []byte
	ContentType   string
	ContentLength int64
}

type fileCacheMeta struct {
	ContentType   string `json:"contentType"`
	ContentLength int64  `json:"contentLength"`
	Encoding      string `json:"encoding"`
	Checksum      string `json:"checksum"`
}

// GetFileCache reads cached file if exists.
func GetFileCache(ctx context.Context, rdb RedisStore, path string) (*FileCacheItem, error) {
	vals, err := rdb.MGet(ctx, fileCacheKey(path), metaCacheKey(path)).Result()
	if err != nil {
		return nil, err
	}
	if len(vals) != 2 || vals[0] == nil || vals[1] == nil {
		return nil, nil
	}
	bodyStr, ok1 := vals[0].(string)
	metaStr, ok2 := vals[1].(string)
	if !ok1 || !ok2 {
		return nil, nil
	}
	var meta fileCacheMeta
	if err := json.Unmarshal([]byte(metaStr), &meta); err != nil {
		return nil, nil
	}
	if meta.Encoding != "base64" {
		return nil, nil
	}
	buf, err := base64.StdEncoding.DecodeString(bodyStr)
	if err != nil {
		return nil, nil
	}
	return &FileCacheItem{
		Buffer:        buf,
		ContentType:   meta.ContentType,
		ContentLength: meta.ContentLength,
	}, nil
}

// SetFileCache writes cached file to Redis.
func SetFileCache(ctx context.Context, rdb RedisStore, path string, item *FileCacheItem) error {
	b64 := base64.StdEncoding.EncodeToString(item.Buffer)
	h := sha1.Sum(item.Buffer)
	meta := fileCacheMeta{
		ContentType:   item.ContentType,
		ContentLength: item.ContentLength,
		Encoding:      "base64",
		Checksum:      hex.EncodeToString(h[:]),
	}
	metaBytes, _ := json.Marshal(&meta)
	pipe := rdb.TxPipeline()
	pipe.SetEx(ctx, fileCacheKey(path), b64, fileCacheTTL)
	pipe.SetEx(ctx, metaCacheKey(path), string(metaBytes), fileCacheTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// ClearFileCache removes cache keys.
func ClearFileCache(ctx context.Context, rdb RedisStore, path string) error {
	if err := rdb.Unlink(ctx, fileCacheKey(path), metaCacheKey(path)).Err(); err == nil {
		return nil
	}
	return rdb.Del(ctx, fileCacheKey(path), metaCacheKey(path)).Err()
}
