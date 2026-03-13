package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"post-go/internal/core"
	"post-go/internal/s3"
	"post-go/internal/utils"

	"github.com/redis/go-redis/v9"
)

type fakeRedisStore struct {
	getResults   map[string]fakeStringResult
	setErr       error
	setExErr     error
	delErr       error
	unlinkErr    error
	scanKeys     []string
	scanCursor   uint64
	ttlResult    time.Duration
	ttlErr       error
	mgetResults  []any
	mgetErr      error
	lastSetKey   string
	lastSetValue string
	lastSetTTL   time.Duration
}

type fakeStringResult struct {
	value string
	err   error
}

func (f *fakeRedisStore) Get(ctx context.Context, key string) *redis.StringCmd {
	result, ok := f.getResults[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(result.value, result.err)
}

func (f *fakeRedisStore) Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	f.lastSetKey = key
	f.lastSetValue, _ = value.(string)
	f.lastSetTTL = expiration
	return redis.NewStatusResult("OK", f.setErr)
}

func (f *fakeRedisStore) SetEx(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	f.lastSetKey = key
	f.lastSetValue, _ = value.(string)
	f.lastSetTTL = expiration
	return redis.NewStatusResult("OK", f.setExErr)
}

func (f *fakeRedisStore) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return redis.NewIntResult(int64(len(keys)), f.delErr)
}

func (f *fakeRedisStore) Unlink(ctx context.Context, keys ...string) *redis.IntCmd {
	return redis.NewIntResult(int64(len(keys)), f.unlinkErr)
}

func (f *fakeRedisStore) Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd {
	return redis.NewScanCmdResult(f.scanKeys, f.scanCursor, nil)
}

func (f *fakeRedisStore) TTL(ctx context.Context, key string) *redis.DurationCmd {
	return redis.NewDurationResult(f.ttlResult, f.ttlErr)
}

func (f *fakeRedisStore) MGet(ctx context.Context, keys ...string) *redis.SliceCmd {
	return redis.NewSliceResult(f.mgetResults, f.mgetErr)
}

func (f *fakeRedisStore) TxPipeline() redis.Pipeliner {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}).TxPipeline()
}

func TestHandleJSONCreateReturnsInternalErrorWhenRedisSetFails(t *testing.T) {
	handler := newTestHandler(&fakeRedisStore{setErr: errors.New("write failed")})
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Code != "internal" {
		t.Fatalf("expected internal error code, got %q", body.Code)
	}
}

func TestHandleJSONCreateStoresValueOnSuccess(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello","path":"note"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if store.lastSetKey != "surl:note" {
		t.Fatalf("expected key surl:note, got %q", store.lastSetKey)
	}
	if store.lastSetValue != "text:hello" {
		t.Fatalf("expected stored value text:hello, got %q", store.lastSetValue)
	}
}

func TestHandleDeleteReturnsInternalErrorWhenRedisDeleteFails(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:note": {value: "text:hello"},
		},
		delErr: errors.New("delete failed"),
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodDelete, "/", strings.NewReader(`{"path":"note"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleDelete(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Code != "internal" {
		t.Fatalf("expected internal error code, got %q", body.Code)
	}
}

func TestHandleDeleteReturnsSuccessAfterRedisDelete(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:note": {value: "text:hello"},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodDelete, "/", strings.NewReader(`{"path":"note"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleDelete(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
}

func newTestHandler(store redisStore) *Handler {
	return &Handler{
		Cfg: core.AppConfig{
			RedisURL:     "redis://unit-test",
			SecretKey:    "secret",
			MaxContentKB: 500,
			MaxFileMB:    10,
		},
		deps: handlerDependencies{
			getRedisStore: func(url string) (redisStore, error) {
				return store, nil
			},
			newFileStore: func(conf s3.Config) (fileObjectStore, error) {
				return nil, errors.New("not implemented")
			},
			clearFileCache: func(ctx context.Context, rdb redisStore, path string) error {
				return nil
			},
			getFileCache: func(ctx context.Context, rdb redisStore, path string) (*core.FileCacheItem, error) {
				return nil, nil
			},
			setFileCache: func(ctx context.Context, rdb redisStore, path string, item *core.FileCacheItem) error {
				return nil
			},
		},
	}
}

func decodeErrorPayload(t *testing.T, response *httptest.ResponseRecorder) utils.ErrorPayload {
	t.Helper()

	var payload utils.ErrorPayload
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode error payload: %v", err)
	}
	return payload
}
