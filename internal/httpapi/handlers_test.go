package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"post-go/internal/core"
	"post-go/internal/s3"
	"post-go/internal/utils"

	"github.com/minio/minio-go/v7"
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

type fakeFileStore struct {
	uploadObjectKey string
	uploadErr       error
	deleteErr       error
	deleteCalls     []string
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

func (f *fakeFileStore) UploadFile(ctx context.Context, filename string, size int64, contentType string, reader io.Reader, ttlSeconds int64) (string, error) {
	if f.uploadErr != nil {
		return "", f.uploadErr
	}
	if f.uploadObjectKey == "" {
		return "post/default/object.txt", nil
	}
	return f.uploadObjectKey, nil
}

func (f *fakeFileStore) GetObject(ctx context.Context, objectKey string) (*minio.Object, minio.ObjectInfo, error) {
	return nil, minio.ObjectInfo{}, errors.New("not implemented")
}

func (f *fakeFileStore) DeleteObject(ctx context.Context, objectKey string) error {
	f.deleteCalls = append(f.deleteCalls, objectKey)
	return f.deleteErr
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
	expected := `{"type":"text","content":"hello"}`
	if store.lastSetValue != expected {
		t.Fatalf("expected stored value %s, got %q", expected, store.lastSetValue)
	}
}

func TestHandleJSONCreateTrimsURLContentWhenTypeIsURL(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"  https://example.com/path?q=1  ","path":"note","type":"url"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	expected := `{"type":"url","content":"https://example.com/path?q=1"}`
	if store.lastSetValue != expected {
		t.Fatalf("expected trimmed url content %s, got %q", expected, store.lastSetValue)
	}
}

func TestHandleJSONCreateRejectsInvalidURLWhenTypeIsURL(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"example.com/path","path":"note","type":"url"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "invalid url value: scheme is required" {
		t.Fatalf("expected invalid url error, got %q", body.Error)
	}
}

func TestHandleJSONCreateRejectsInvalidURLWhenConvertIsURL(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"not a valid uri","path":"note","convert":"url"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "invalid url value: scheme is required" {
		t.Fatalf("expected invalid url error, got %q", body.Error)
	}
}

func TestHandleDeleteReturnsInternalErrorWhenRedisDeleteFails(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:note": {value: `{"type":"text","content":"hello"}`},
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
			"surl:note": {value: `{"type":"text","content":"hello"}`},
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

func TestHandleFileUploadDeletesObjectWhenRedisWriteFails(t *testing.T) {
	store := &fakeRedisStore{setErr: errors.New("write failed")}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded.txt"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequest(t, http.MethodPost, map[string]string{}, "note.txt", "hello")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.Code)
	}
	if len(fileStore.deleteCalls) != 1 || fileStore.deleteCalls[0] != "post/default/uploaded.txt" {
		t.Fatalf("expected compensation delete for uploaded object, got %+v", fileStore.deleteCalls)
	}
}

func TestHandleFileUploadStoresObjectOnSuccess(t *testing.T) {
	store := &fakeRedisStore{}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded.txt"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequest(t, http.MethodPost, map[string]string{"path": "note"}, "note.txt", "hello")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if store.lastSetKey != "surl:note.txt" {
		t.Fatalf("expected stored key surl:note.txt, got %q", store.lastSetKey)
	}
	expected := `{"type":"file","content":"post/default/uploaded.txt"}`
	if store.lastSetValue != expected {
		t.Fatalf("expected stored value for uploaded object %s, got %q", expected, store.lastSetValue)
	}
	if len(fileStore.deleteCalls) != 0 {
		t.Fatalf("expected no compensation delete, got %+v", fileStore.deleteCalls)
	}
}

func TestHandleFileUploadAppendsFilenameExtensionToPath(t *testing.T) {
	store := &fakeRedisStore{}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded.txt"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequest(t, http.MethodPost, map[string]string{"path": "custom/path"}, "note.txt", "hello")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if store.lastSetKey != "surl:custom/path.txt" {
		t.Fatalf("expected path to include uploaded file extension, got %q", store.lastSetKey)
	}
}

func TestHandleJSONCreateStoresTitleInJSONValue(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello","path":"note","title":"Greeting"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	expected := `{"type":"text","content":"hello","title":"Greeting"}`
	if store.lastSetValue != expected {
		t.Fatalf("expected stored value %s, got %q", expected, store.lastSetValue)
	}
}

func TestHandleFileUploadStoresTitleInJSONValue(t *testing.T) {
	store := &fakeRedisStore{}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded.txt"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequest(t, http.MethodPost, map[string]string{"path": "note", "title": "Attachment"}, "note.txt", "hello")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	expected := `{"type":"file","content":"post/default/uploaded.txt","title":"Attachment"}`
	if store.lastSetValue != expected {
		t.Fatalf("expected stored value %s, got %q", expected, store.lastSetValue)
	}
}

func TestHandleFileUploadReturnsMainFailureWhenCompensationFails(t *testing.T) {
	store := &fakeRedisStore{setErr: errors.New("write failed")}
	fileStore := &fakeFileStore{
		uploadObjectKey: "post/default/uploaded.txt",
		deleteErr:       errors.New("cleanup failed"),
	}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequest(t, http.MethodPost, map[string]string{}, "note.txt", "hello")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.Code)
	}
	if len(fileStore.deleteCalls) != 1 {
		t.Fatalf("expected compensation delete attempt, got %+v", fileStore.deleteCalls)
	}
}

func newTestHandler(store redisStore) *Handler {
	return newTestHandlerWithDeps(store, &fakeFileStore{})
}

func newTestHandlerWithDeps(store redisStore, fileStore fileObjectStore) *Handler {
	return &Handler{
		Cfg: core.AppConfig{
			RedisURL:       "redis://unit-test",
			SecretKey:      "secret",
			MaxContentKB:   500,
			MaxFileMB:      10,
			S3Endpoint:     "https://s3.example.com",
			S3AccessKeyID:  "key",
			S3SecretAccess: "secret",
			S3Bucket:       "bucket",
			S3Region:       "auto",
		},
		deps: handlerDependencies{
			getRedisStore: func(url string) (redisStore, error) {
				return store, nil
			},
			newFileStore: func(conf s3.Config) (fileObjectStore, error) {
				return fileStore, nil
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

func newMultipartUploadRequest(t *testing.T, method string, fields map[string]string, filename string, content string) *http.Request {
	t.Helper()

	body := &strings.Builder{}
	writer := multipart.NewWriter(body)
	fileWriter, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("failed to create file part: %v", err)
	}
	if _, err := fileWriter.Write([]byte(content)); err != nil {
		t.Fatalf("failed to write file content: %v", err)
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("failed to write field: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close multipart writer: %v", err)
	}

	request := httptest.NewRequest(method, "/", strings.NewReader(body.String()))
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func decodeErrorPayload(t *testing.T, response *httptest.ResponseRecorder) utils.ErrorPayload {
	t.Helper()

	var payload utils.ErrorPayload
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode error payload: %v", err)
	}
	return payload
}
