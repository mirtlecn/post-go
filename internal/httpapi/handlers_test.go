package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"post-go/internal/core"
	"post-go/internal/s3"
	"post-go/internal/storage"
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
	existsResult int64
	existsErr    error
	scanKeys     []string
	scanCursor   uint64
	ttlResult    time.Duration
	ttlErr       error
	mgetResults  []any
	mgetErr      error
	mgetCalls    [][]string
	zaddErr      error
	zremErr      error
	zremKeys     []string
	zremMembers  []any
	zcardResult  int64
	zcardErr     error
	zrangeResult []redis.Z
	zrangeErr    error
	zaddKeys     []string
	zaddMembers  []redis.Z
	lastSetKey   string
	lastSetValue string
	lastSetTTL   time.Duration
	setKeys      []string
	setValues    []string
}

type fakeStringResult struct {
	value string
	err   error
}

type fakeFileStore struct {
	uploadObjectKey string
	uploadErr       error
	lastUploadTTL   int64
	lastUploadType  string
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
	f.setKeys = append(f.setKeys, key)
	f.setValues = append(f.setValues, f.lastSetValue)
	if f.getResults == nil {
		f.getResults = map[string]fakeStringResult{}
	}
	f.getResults[key] = fakeStringResult{value: f.lastSetValue}
	return redis.NewStatusResult("OK", f.setErr)
}

func (f *fakeRedisStore) SetEx(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	f.lastSetKey = key
	f.lastSetValue, _ = value.(string)
	f.lastSetTTL = expiration
	f.setKeys = append(f.setKeys, key)
	f.setValues = append(f.setValues, f.lastSetValue)
	if f.getResults == nil {
		f.getResults = map[string]fakeStringResult{}
	}
	f.getResults[key] = fakeStringResult{value: f.lastSetValue}
	return redis.NewStatusResult("OK", f.setExErr)
}

func (f *fakeRedisStore) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	if f.getResults != nil {
		for _, key := range keys {
			delete(f.getResults, key)
		}
	}
	return redis.NewIntResult(int64(len(keys)), f.delErr)
}

func (f *fakeRedisStore) Unlink(ctx context.Context, keys ...string) *redis.IntCmd {
	if f.getResults != nil {
		for _, key := range keys {
			delete(f.getResults, key)
		}
	}
	return redis.NewIntResult(int64(len(keys)), f.unlinkErr)
}

func (f *fakeRedisStore) Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd {
	return redis.NewScanCmdResult(f.scanKeys, f.scanCursor, nil)
}

func (f *fakeRedisStore) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	return redis.NewIntResult(f.existsResult, f.existsErr)
}

func (f *fakeRedisStore) TTL(ctx context.Context, key string) *redis.DurationCmd {
	return redis.NewDurationResult(f.ttlResult, f.ttlErr)
}

func (f *fakeRedisStore) MGet(ctx context.Context, keys ...string) *redis.SliceCmd {
	f.mgetCalls = append(f.mgetCalls, append([]string(nil), keys...))
	if f.mgetResults != nil || f.mgetErr != nil {
		return redis.NewSliceResult(f.mgetResults, f.mgetErr)
	}
	results := make([]any, len(keys))
	for index, key := range keys {
		result, ok := f.getResults[key]
		if !ok || result.err == redis.Nil {
			results[index] = nil
			continue
		}
		if result.err != nil {
			return redis.NewSliceResult(nil, result.err)
		}
		results[index] = result.value
	}
	return redis.NewSliceResult(results, nil)
}

func (f *fakeRedisStore) TxPipeline() redis.Pipeliner {
	return redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}).TxPipeline()
}

func (f *fakeRedisStore) ZAdd(ctx context.Context, key string, members ...redis.Z) *redis.IntCmd {
	f.zaddKeys = append(f.zaddKeys, key)
	f.zaddMembers = append(f.zaddMembers, members...)
	return redis.NewIntResult(int64(len(members)), f.zaddErr)
}

func (f *fakeRedisStore) ZRem(ctx context.Context, key string, members ...any) *redis.IntCmd {
	f.zremKeys = append(f.zremKeys, key)
	f.zremMembers = append(f.zremMembers, members...)
	return redis.NewIntResult(int64(len(members)), f.zremErr)
}

func (f *fakeRedisStore) ZRevRangeWithScores(ctx context.Context, key string, start, stop int64) *redis.ZSliceCmd {
	return redis.NewZSliceCmdResult(f.zrangeResult, f.zrangeErr)
}

func (f *fakeRedisStore) ZCard(ctx context.Context, key string) *redis.IntCmd {
	return redis.NewIntResult(f.zcardResult, f.zcardErr)
}

func (f *fakeFileStore) UploadFile(ctx context.Context, filename string, size int64, contentType string, reader io.Reader, ttlSeconds int64) (string, error) {
	f.lastUploadTTL = ttlSeconds
	f.lastUploadType = contentType
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
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Type != "text" || stored.Content != "hello" {
		t.Fatalf("unexpected stored value: %+v", stored)
	}
	assertRFC3339Value(t, stored.Created)
	var body CreateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Title != "" {
		t.Fatalf("expected empty title, got %q", body.Title)
	}
	assertRFC3339Value(t, body.Created)
}

func TestHandleJSONCreateNormalizesPathBeforeStore(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello","path":"/note/"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if store.lastSetKey != "surl:note" {
		t.Fatalf("expected normalized key surl:note, got %q", store.lastSetKey)
	}
}

func TestHandleJSONCreateStoresWithoutExpirationWhenTTLIsZero(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello","path":"note","ttl":0}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if store.lastSetTTL != 0 {
		t.Fatalf("expected no expiration, got %v", store.lastSetTTL)
	}
	var body CreateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.TTL != nil {
		t.Fatalf("expected ttl to be null, got %+v", body.TTL)
	}
	if body.Warning != "" {
		t.Fatalf("expected no warning, got %q", body.Warning)
	}
}

func TestHandleJSONCreateNormalizesProvidedCreatedValue(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello","path":"note","created":"2022.10.11"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Created != "2022-10-10T16:00:00Z" {
		t.Fatalf("expected normalized created, got %q", stored.Created)
	}
	var body CreateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Created != "2022-10-10T16:00:00Z" {
		t.Fatalf("expected response created, got %+v", body)
	}
}

func TestHandleJSONCreateRejectsInvalidCreatedValue(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello","path":"note","created":"2012.01.11 09"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "`created` has invalid format" {
		t.Fatalf("unexpected error payload: %+v", body)
	}
}

func TestHandleJSONCreateRejectsNonNaturalTTL(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello","path":"note","ttl":1.5}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "`ttl` must be a natural number" {
		t.Fatalf("unexpected error payload: %+v", body)
	}
}

func TestHandleJSONCreateRejectsStringTTL(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello","path":"note","ttl":"10"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "`ttl` must be a natural number" {
		t.Fatalf("unexpected error payload: %+v", body)
	}
}

func TestHandleJSONCreateRejectsTTLAboveBusinessLimit(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello","path":"note","ttl":525601}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	if len(store.setKeys) != 0 {
		t.Fatalf("expected no redis write, got %+v", store.setKeys)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "`ttl` must be between 0 and 525600 minutes" {
		t.Fatalf("unexpected error payload: %+v", body)
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
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Type != "url" || stored.Content != "https://example.com/path?q=1" {
		t.Fatalf("unexpected stored value: %+v", stored)
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

func TestHandleJSONCreateRejectsMismatchedTypeAndConvert(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello","path":"note","type":"text","convert":"html"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "`type` and `convert` must match when both are provided" {
		t.Fatalf("unexpected error payload: %+v", body)
	}
}

func TestHandleJSONCreateCreatesTopicHome(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"path":"anime","type":"topic"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if store.lastSetKey != "surl:anime" {
		t.Fatalf("expected topic home to be stored, got %q", store.lastSetKey)
	}
	if len(store.zaddKeys) == 0 || store.zaddKeys[0] != "topic:anime:items" {
		t.Fatalf("expected topic items key to be created, got %+v", store.zaddKeys)
	}
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Type != topicType {
		t.Fatalf("expected stored topic type, got %q", stored.Type)
	}
	if stored.Title != "anime" {
		t.Fatalf("expected topic title fallback to path, got %q", stored.Title)
	}
	assertRFC3339Value(t, stored.Created)
}

func TestHandleJSONCreateStoresTopicTitle(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"path":"anime","type":"topic","title":"Anime Archive"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Title != "Anime Archive" {
		t.Fatalf("expected stored topic title, got %q", stored.Title)
	}
	var body CreateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Title != "Anime Archive" {
		t.Fatalf("expected response topic title, got %+v", body)
	}
}

func TestHandleJSONUpdatePreservesTopicTitleWhenTitleOmitted(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"Anime Archive","created":"2022-10-11T01:11:01Z"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"path":"anime","type":"topic"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, true)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Title != "Anime Archive" {
		t.Fatalf("expected preserved topic title, got %q", stored.Title)
	}
	if stored.Created != "2022-10-11T01:11:01Z" {
		t.Fatalf("expected preserved topic created, got %q", stored.Created)
	}
}

func TestHandleJSONUpdateOverridesTopicTitleWhenProvided(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"Anime Archive","created":"2022-10-11T01:11:01Z"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"path":"anime","type":"topic","title":"Anime Notes"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, true)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Title != "Anime Notes" {
		t.Fatalf("expected updated topic title, got %q", stored.Title)
	}
	if stored.Created != "2022-10-11T01:11:01Z" {
		t.Fatalf("expected preserved topic created, got %q", stored.Created)
	}
}

func TestHandleJSONUpdatePreservesCreatedWhenOmitted(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:note": {value: `{"type":"text","content":"hello","created":"2022-10-11T01:11:01Z"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(`{"url":"updated","path":"note"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, true)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Created != "2022-10-11T01:11:01Z" {
		t.Fatalf("expected preserved created, got %q", stored.Created)
	}
	var body CreateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Created != "2022-10-11T01:11:01Z" {
		t.Fatalf("expected response created, got %+v", body)
	}
}

func TestHandleJSONCreateRejectsTTLForTopic(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"path":"anime","type":"topic","ttl":10}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "topic does not support ttl" {
		t.Fatalf("unexpected error payload: %+v", body)
	}
}

func TestHandleJSONCreateStoresTopicItemAndRebuildsIndex(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"anime"}`},
		},
		zrangeResult: []redis.Z{{Score: float64(time.Date(2026, time.December, 23, 10, 0, 0, 0, time.UTC).Unix()), Member: "castle"}},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"topic":"anime","path":"castle","url":"# Castle","type":"md2html","title":"Castle"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if len(store.setKeys) < 2 {
		t.Fatalf("expected item write and topic rebuild, got %+v", store.setKeys)
	}
	if store.setKeys[0] != "surl:anime/castle" {
		t.Fatalf("expected topic item write first, got %+v", store.setKeys)
	}
	if store.setKeys[len(store.setKeys)-1] != "surl:anime" {
		t.Fatalf("expected topic home rebuild, got %+v", store.setKeys)
	}
}

func TestHandleJSONCreateRollsBackTopicItemWhenTopicSyncFails(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"anime"}`},
		},
		zaddErr: errors.New("zadd failed"),
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"topic":"anime","path":"castle","url":"# Castle","type":"md2html","title":"Castle"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.Code)
	}
	if _, ok := store.getResults["surl:anime/castle"]; ok {
		t.Fatalf("expected topic item key to be rolled back, got %+v", store.getResults)
	}
}

func TestHandleJSONCreateRejectsSlashOnlyPathWhenTopicProvided(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"anime"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"topic":"anime","path":"///","url":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "`path` must not be \"/\" when `topic` is provided" {
		t.Fatalf("unexpected error payload: %+v", body)
	}
}

func TestHandleJSONCreateRejectsEmptyTopicMemberSegment(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"anime"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"topic":"anime","path":"anime//castle","url":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "`path` must not contain empty topic members" {
		t.Fatalf("unexpected error payload: %+v", body)
	}
}

func TestResolveTopicPathUsesLongestExistingTopicPrefix(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:blog":      {value: `{"type":"topic","content":"<html></html>","title":"blog"}`},
			"surl:blog/2026": {value: `{"type":"topic","content":"<html></html>","title":"blog/2026"}`},
		},
	}
	handler := newTestHandler(store)

	resolved, err := handler.resolveTopicPath(context.Background(), store, "", "blog/2026/post-1")
	if err != nil {
		t.Fatalf("expected resolve to succeed, got %v", err)
	}
	if !resolved.IsTopicItem {
		t.Fatalf("expected topic item, got %+v", resolved)
	}
	if resolved.TopicName != "blog/2026" {
		t.Fatalf("expected nested topic prefix, got %+v", resolved)
	}
	if resolved.RelativePath != "post-1" {
		t.Fatalf("expected relative path post-1, got %+v", resolved)
	}
}

func TestResolveTopicPathFallsBackToShorterTopicPrefix(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:blog": {value: `{"type":"topic","content":"<html></html>","title":"blog"}`},
		},
	}
	handler := newTestHandler(store)

	resolved, err := handler.resolveTopicPath(context.Background(), store, "", "blog/2027/post-1")
	if err != nil {
		t.Fatalf("expected resolve to succeed, got %v", err)
	}
	if resolved.TopicName != "blog" || resolved.RelativePath != "2027/post-1" {
		t.Fatalf("unexpected resolved path: %+v", resolved)
	}
}

func TestResolveTopicPathNormalizesTrailingSlashForTopicItem(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"anime"}`},
		},
	}
	handler := newTestHandler(store)

	resolved, err := handler.resolveTopicPath(context.Background(), store, "anime", "/castle/")
	if err != nil {
		t.Fatalf("expected resolve to succeed, got %v", err)
	}
	if resolved.FullPath != "anime/castle" || resolved.RelativePath != "castle" {
		t.Fatalf("unexpected resolved path: %+v", resolved)
	}
}

func TestHandlePathNormalizesTrailingSlashOnLookup(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:note": {value: `{"type":"text","content":"hello"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/note/", nil)
	response := httptest.NewRecorder()

	handler.handlePath(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	body := strings.TrimSpace(response.Body.String())
	if body != "hello" {
		t.Fatalf("expected lookup content hello, got %q", body)
	}
}

func TestRebuildTopicIndexRemovesStaleMembers(t *testing.T) {
	store := &fakeRedisStore{
		zrangeResult: []redis.Z{
			{Score: float64(time.Date(2026, time.December, 23, 10, 0, 0, 0, time.UTC).Unix()), Member: "alive"},
			{Score: float64(time.Date(2026, time.December, 22, 10, 0, 0, 0, time.UTC).Unix()), Member: "gone"},
			{Score: 0, Member: topicPlaceholderMember},
		},
		getResults: map[string]fakeStringResult{
			"surl:anime":       {value: `{"type":"topic","content":"<html></html>","title":"Anime Archive"}`},
			"surl:anime/alive": {value: `{"type":"text","content":"hello","title":"Alive"}`},
		},
	}
	handler := newTestHandler(store)

	if err := handler.rebuildTopicIndex(context.Background(), store, "anime"); err != nil {
		t.Fatalf("expected rebuild to succeed, got %v", err)
	}
	if len(store.mgetCalls) != 1 {
		t.Fatalf("expected one mget call, got %+v", store.mgetCalls)
	}
	expectedMGetKeys := []string{"surl:anime/alive", "surl:anime/gone"}
	if strings.Join(store.mgetCalls[0], ",") != strings.Join(expectedMGetKeys, ",") {
		t.Fatalf("expected topic rebuild mget keys %v, got %v", expectedMGetKeys, store.mgetCalls[0])
	}
	if len(store.zremKeys) != 1 || store.zremKeys[0] != "topic:anime:items" {
		t.Fatalf("expected stale zrem on topic items, got keys=%+v members=%+v", store.zremKeys, store.zremMembers)
	}
	if len(store.zremMembers) != 1 || store.zremMembers[0] != "gone" {
		t.Fatalf("expected stale member removal, got %+v", store.zremMembers)
	}
	if store.lastSetKey != "surl:anime" {
		t.Fatalf("expected rebuilt topic home, got %q", store.lastSetKey)
	}
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Title != "Anime Archive" {
		t.Fatalf("expected rebuilt topic title to be preserved, got %q", stored.Title)
	}
}

func TestRebuildTopicIndexPrefersStoredCreatedForSorting(t *testing.T) {
	store := &fakeRedisStore{
		zrangeResult: []redis.Z{
			{Score: float64(time.Date(2026, time.December, 23, 10, 0, 0, 0, time.UTC).Unix()), Member: "old"},
			{Score: float64(time.Date(2026, time.December, 22, 10, 0, 0, 0, time.UTC).Unix()), Member: "new"},
		},
		getResults: map[string]fakeStringResult{
			"surl:anime":     {value: `{"type":"topic","content":"<html></html>","title":"Anime Archive"}`},
			"surl:anime/old": {value: `{"type":"text","content":"hello","title":"Old","created":"2022-10-11T01:11:01Z"}`},
			"surl:anime/new": {value: `{"type":"text","content":"hello","title":"New","created":"2023-10-11T01:11:01Z"}`},
		},
	}
	handler := newTestHandler(store)

	if err := handler.rebuildTopicIndex(context.Background(), store, "anime"); err != nil {
		t.Fatalf("expected rebuild to succeed, got %v", err)
	}
	stored := storage.ParseStoredValue(store.lastSetValue)
	if strings.Index(stored.Content, ">New<") >= strings.Index(stored.Content, ">Old<") {
		t.Fatalf("expected topic index to sort by created, got %q", stored.Content)
	}
}

func TestHandleJSONCreateConflictIncludesExistingTitle(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:note": {value: `{"type":"text","content":"hello","title":"Greeting"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"updated","path":"note"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d", response.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	details, ok := body["details"].(map[string]any)
	if !ok {
		t.Fatalf("expected details map, got %#v", body["details"])
	}
	existing, ok := details["existing"].(map[string]any)
	if !ok {
		t.Fatalf("expected existing map, got %#v", details["existing"])
	}
	if existing["title"] != "Greeting" {
		t.Fatalf("expected existing title Greeting, got %#v", existing["title"])
	}
}

func TestHandleLookupAuthedFromBodyReturnsTopicSummary(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"Anime Archive","created":"2022-10-11T01:11:01Z"}`},
		},
		zcardResult: 4,
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(`{"path":"anime","type":"topic"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	if !handler.handleLookupAuthedFromBody(response, request) {
		t.Fatalf("expected topic lookup to be handled")
	}
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body ItemResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Type != topicType || body.Content != "3" {
		t.Fatalf("unexpected topic response: %+v", body)
	}
	if body.Title != "Anime Archive" {
		t.Fatalf("expected topic title Anime Archive, got %+v", body)
	}
	if body.Created != "2022-10-11T01:11:01Z" {
		t.Fatalf("expected topic created, got %+v", body)
	}
}

func TestHandleLookupAuthedFromBodyReturnsTopicList(t *testing.T) {
	store := &fakeRedisStore{
		scanKeys: []string{"topic:anime:items", "topic:blog:items"},
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"Anime Archive","created":"2022-10-11T01:11:01Z"}`},
			"surl:blog":  {value: `{"type":"topic","content":"<html></html>"}`},
		},
		zcardResult: 1,
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(`{"type":"topic"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	if !handler.handleLookupAuthedFromBody(response, request) {
		t.Fatalf("expected topic list lookup to be handled")
	}
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body []ItemResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(body) != 2 {
		t.Fatalf("expected 2 topics, got %+v", body)
	}
	if len(store.mgetCalls) != 1 {
		t.Fatalf("expected one mget call, got %+v", store.mgetCalls)
	}
	expectedMGetKeys := []string{"surl:anime", "surl:blog"}
	if strings.Join(store.mgetCalls[0], ",") != strings.Join(expectedMGetKeys, ",") {
		t.Fatalf("expected topic list mget keys %v, got %v", expectedMGetKeys, store.mgetCalls[0])
	}
	if body[0].Type != topicType || body[1].Type != topicType {
		t.Fatalf("unexpected topic list response: %+v", body)
	}
	if body[0].Title != "Anime Archive" || body[1].Title != "blog" {
		t.Fatalf("expected topic titles, got %+v", body)
	}
	if body[0].Created != "2022-10-11T01:11:01Z" || body[1].Created != "illegal" {
		t.Fatalf("expected topic created values, got %+v", body)
	}
}

func TestHandleListUsesMGetForStoredValues(t *testing.T) {
	store := &fakeRedisStore{
		scanKeys: []string{"surl:note", "surl:anime"},
		getResults: map[string]fakeStringResult{
			"surl:note":  {value: `{"type":"text","content":"hello","title":"Greeting","created":"2023-10-11T01:11:01Z"}`},
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"Anime Archive","created":"2022-10-11T01:11:01Z"}`},
		},
		ttlResult:   0,
		zcardResult: 3,
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handler.handleList(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if len(store.mgetCalls) != 1 {
		t.Fatalf("expected one mget call, got %+v", store.mgetCalls)
	}
	expectedMGetKeys := []string{"surl:note", "surl:anime"}
	if strings.Join(store.mgetCalls[0], ",") != strings.Join(expectedMGetKeys, ",") {
		t.Fatalf("expected list mget keys %v, got %v", expectedMGetKeys, store.mgetCalls[0])
	}
	var body []ItemResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(body) != 2 {
		t.Fatalf("expected 2 items, got %+v", body)
	}
	if body[0].Path != "note" || body[0].Content != "hello" {
		t.Fatalf("unexpected first item: %+v", body[0])
	}
	if body[1].Path != "anime" || body[1].Type != topicType || body[1].Content != "2" {
		t.Fatalf("unexpected topic item: %+v", body[1])
	}
	if body[0].Created != "2023-10-11T01:11:01Z" || body[1].Created != "2022-10-11T01:11:01Z" {
		t.Fatalf("expected created values, got %+v", body)
	}
}

func TestHandleListReturnsIllegalCreatedWhenStoredValueMissingCreated(t *testing.T) {
	store := &fakeRedisStore{
		scanKeys: []string{"surl:note"},
		getResults: map[string]fakeStringResult{
			"surl:note": {value: `{"type":"text","content":"hello","title":"Greeting"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()

	handler.handleList(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body []ItemResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(body) != 1 || body[0].Created != "illegal" {
		t.Fatalf("expected illegal created, got %+v", body)
	}
}

func TestHandleJSONCreateStoresRawTopicMarkdown(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"Anime Archive"}`},
		},
		zrangeResult: []redis.Z{{Score: float64(time.Date(2026, time.December, 23, 10, 0, 0, 0, time.UTC).Unix()), Member: "castle"}},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"topic":"anime","path":"castle","url":"# Castle","type":"md2html","title":"Castle Notes"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	stored := storage.ParseStoredValue(store.setValues[0])
	if stored.Type != "md" {
		t.Fatalf("expected md type, got %q", stored.Type)
	}
	if stored.Content != "# Castle" {
		t.Fatalf("expected raw markdown content, got %q", stored.Content)
	}
}

func TestHandleLookupAuthedReturnsTTLForExpiringItem(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:note": {value: `{"type":"url","content":"https://example.com","title":"Greeting","created":"2022-10-11T01:11:01Z"}`},
		},
		ttlResult: 3 * time.Minute,
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(`{"path":"note"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	if !handler.handleLookupAuthedFromBody(response, request) {
		t.Fatalf("expected lookup to be handled")
	}
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	var body ItemResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.TTL == nil || *body.TTL != 3 {
		t.Fatalf("expected ttl 3, got %+v", body)
	}
	if body.Created != "2022-10-11T01:11:01Z" {
		t.Fatalf("expected created value, got %+v", body)
	}
}

func TestHandleDeleteRejectsTopicHomeWithoutTopicType(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"anime"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodDelete, "/", strings.NewReader(`{"path":"anime"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleDelete(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
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
			"surl:note": {value: `{"type":"text","content":"hello","title":"Greeting","created":"2022-10-11T01:11:01Z"}`},
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
	var body DeleteResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Title != "Greeting" {
		t.Fatalf("expected delete title Greeting, got %+v", body)
	}
	if body.Created != "2022-10-11T01:11:01Z" {
		t.Fatalf("expected delete created value, got %+v", body)
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
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Type != "file" || stored.Content != "post/default/uploaded.txt" {
		t.Fatalf("unexpected stored value: %+v", stored)
	}
	assertRFC3339Value(t, stored.Created)
	if len(fileStore.deleteCalls) != 0 {
		t.Fatalf("expected no compensation delete, got %+v", fileStore.deleteCalls)
	}
	if fileStore.lastUploadType != "text/plain; charset=utf-8" {
		t.Fatalf("expected inferred text/plain content type, got %q", fileStore.lastUploadType)
	}
	var body CreateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Title != "" {
		t.Fatalf("expected empty title, got %q", body.Title)
	}
	assertRFC3339Value(t, body.Created)
}

func TestHandleFileUploadRollsBackWhenTopicSyncFails(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html></html>","title":"anime"}`},
		},
		zaddErr: errors.New("zadd failed"),
	}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded.txt"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequest(t, http.MethodPost, map[string]string{"topic": "anime", "path": "note"}, "note.txt", "hello")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", response.Code)
	}
	if _, ok := store.getResults["surl:anime/note.txt"]; ok {
		t.Fatalf("expected rolled back file key, got %+v", store.getResults)
	}
	if len(fileStore.deleteCalls) != 1 || fileStore.deleteCalls[0] != "post/default/uploaded.txt" {
		t.Fatalf("expected uploaded object cleanup, got %+v", fileStore.deleteCalls)
	}
}

func TestHandleFileUploadStoresWithoutExpirationWhenTTLIsZero(t *testing.T) {
	store := &fakeRedisStore{}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded.txt"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequest(t, http.MethodPost, map[string]string{"path": "note", "ttl": "0"}, "note.txt", "hello")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if store.lastSetTTL != 0 {
		t.Fatalf("expected no expiration, got %v", store.lastSetTTL)
	}
	if fileStore.lastUploadTTL != 0 {
		t.Fatalf("expected upload ttlSeconds 0, got %d", fileStore.lastUploadTTL)
	}
	var body CreateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.TTL != nil {
		t.Fatalf("expected ttl to be null, got %+v", body.TTL)
	}
}

func TestHandleFileUploadRejectsNonNaturalTTL(t *testing.T) {
	store := &fakeRedisStore{}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded.txt"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequest(t, http.MethodPost, map[string]string{"path": "note", "ttl": "1.5"}, "note.txt", "hello")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "`ttl` must be a natural number" {
		t.Fatalf("unexpected error payload: %+v", body)
	}
}

func TestHandleFileUploadRejectsTTLAboveBusinessLimit(t *testing.T) {
	store := &fakeRedisStore{}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded.txt"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequest(t, http.MethodPost, map[string]string{"path": "note", "ttl": "525601"}, "note.txt", "hello")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	if len(store.setKeys) != 0 {
		t.Fatalf("expected no redis write, got %+v", store.setKeys)
	}
	if fileStore.lastUploadTTL != 0 {
		t.Fatalf("expected no upload ttl to be passed, got %d", fileStore.lastUploadTTL)
	}
	body := decodeErrorPayload(t, response)
	if body.Error != "`ttl` must be between 0 and 525600 minutes" {
		t.Fatalf("unexpected error payload: %+v", body)
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

func TestHandleFileUploadKeepsExplicitMultipartContentType(t *testing.T) {
	store := &fakeRedisStore{}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded.txt"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequestWithFileContentType(t, http.MethodPost, map[string]string{"path": "note"}, "note.txt", "hello", "text/plain")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if fileStore.lastUploadType != "text/plain" {
		t.Fatalf("expected explicit multipart content type to be preserved, got %q", fileStore.lastUploadType)
	}
}

func TestHandleFileUploadRepairsOctetStreamUsingExtension(t *testing.T) {
	store := &fakeRedisStore{}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded.pdf"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequestWithFileContentType(t, http.MethodPost, map[string]string{"path": "note"}, "note.pdf", "%PDF-1.7\nbody", "application/octet-stream")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if fileStore.lastUploadType != "application/pdf" {
		t.Fatalf("expected octet-stream to be repaired to application/pdf, got %q", fileStore.lastUploadType)
	}
}

func TestHandleFileUploadDetectsPDFWithoutExtension(t *testing.T) {
	store := &fakeRedisStore{}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequest(t, http.MethodPost, map[string]string{"path": "note"}, "note", "%PDF-1.7\nbody")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if fileStore.lastUploadType != "application/pdf" {
		t.Fatalf("expected PDF content type from body detection, got %q", fileStore.lastUploadType)
	}
}

func TestHandleFileUploadDetectsPNGWithoutExtension(t *testing.T) {
	store := &fakeRedisStore{}
	fileStore := &fakeFileStore{uploadObjectKey: "post/default/uploaded"}
	handler := newTestHandlerWithDeps(store, fileStore)
	request := newMultipartUploadRequest(t, http.MethodPost, map[string]string{"path": "note"}, "note", "\x89PNG\r\n\x1a\npng-body")
	response := httptest.NewRecorder()

	handler.handleFileUpload(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	if fileStore.lastUploadType != "image/png" {
		t.Fatalf("expected PNG content type from body detection, got %q", fileStore.lastUploadType)
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
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Type != "text" || stored.Content != "hello" || stored.Title != "Greeting" {
		t.Fatalf("unexpected stored value: %+v", stored)
	}
	assertRFC3339Value(t, stored.Created)
	var body CreateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Title != "Greeting" {
		t.Fatalf("expected create title Greeting, got %+v", body)
	}
	assertRFC3339Value(t, body.Created)
}

func TestHandleJSONCreateMD2HTMLStoresRawMarkdown(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"# Hello","path":"note","type":"md2html","title":"Greeting"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Type != "md" {
		t.Fatalf("expected stored md type, got %q", stored.Type)
	}
	if stored.Content != "# Hello" {
		t.Fatalf("expected stored raw markdown, got %q", stored.Content)
	}
}

func TestHandleJSONCreateQRCodeStoresRawContent(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"https://example.com/qr","path":"qr","type":"qrcode","title":"QR"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d", response.Code)
	}
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Type != "qrcode" {
		t.Fatalf("expected stored qrcode type, got %q", stored.Type)
	}
	if stored.Content != "https://example.com/qr" {
		t.Fatalf("expected stored raw qrcode content, got %q", stored.Content)
	}
}

func TestServeHTTPRendersStoredMarkdown(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:note": {value: `{"type":"md","content":"# Hello","title":"Greeting"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/note", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if response.Header().Get("Cache-Control") != publicCacheControl {
		t.Fatalf("expected default cache header %q, got %q", publicCacheControl, response.Header().Get("Cache-Control"))
	}
	if !strings.Contains(response.Body.String(), "<title>Greeting</title>") {
		t.Fatalf("expected markdown page title, got %q", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "<h1 id=\"hello\">Hello</h1>") {
		t.Fatalf("expected markdown body to render, got %q", response.Body.String())
	}
}

func TestServeHTTPRendersStoredTopicMarkdownWithBacklink(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime/castle": {value: `{"type":"md","content":"# Castle","title":"Castle Notes"}`},
			"surl:anime":        {value: `{"type":"topic","content":"<html></html>","title":"Anime Archive"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/anime/castle", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `href="/anime"`) {
		t.Fatalf("expected topic backlink href, got %q", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "Anime Archive") {
		t.Fatalf("expected topic backlink label, got %q", response.Body.String())
	}
}

func TestServeHTTPRendersStoredQRCode(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:qr": {value: `{"type":"qrcode","content":"https://example.com/qr","title":"QR"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/qr", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "Scan this QR code") {
		t.Fatalf("expected qrcode text body, got %q", response.Body.String())
	}
}

func TestServeHTTPRejectsDirectEmbeddedAssetAccess(t *testing.T) {
	handler := newTestHandler(&fakeRedisStore{})
	request := httptest.NewRequest(http.MethodGet, "/asset/md-base-7f7c1c5a.css", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Code != "forbidden" {
		t.Fatalf("expected forbidden error payload, got %+v", body)
	}
}

func TestServeHTTPReturnsEmbeddedAssetForSameOriginReferer(t *testing.T) {
	handler := newTestHandler(&fakeRedisStore{})
	request := httptest.NewRequest(http.MethodGet, "/asset/md-base-7f7c1c5a.css", nil)
	request.Host = "example.com"
	request.Header.Set("Referer", "http://example.com/note")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if response.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Fatalf("expected immutable cache header, got %q", response.Header().Get("Cache-Control"))
	}
	if !strings.Contains(response.Header().Get("Content-Type"), "text/css") {
		t.Fatalf("expected css content type, got %q", response.Header().Get("Content-Type"))
	}
	if response.Body.Len() == 0 {
		t.Fatalf("expected asset body")
	}
}

func TestServeHTTPReturnsEmbeddedAssetHeadersForHEAD(t *testing.T) {
	handler := newTestHandler(&fakeRedisStore{})
	request := httptest.NewRequest(http.MethodHead, "/asset/highlight-core-b7ec7622.js", nil)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if response.Body.Len() != 0 {
		t.Fatalf("expected empty body for HEAD, got %q", response.Body.String())
	}
}

func TestServeHTTPRejectsUnsupportedMethodForReservedAssetPath(t *testing.T) {
	handler := newTestHandler(&fakeRedisStore{})
	request := httptest.NewRequest(http.MethodDelete, "/asset/md-base-7f7c1c5a.css", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status 405, got %d", response.Code)
	}
	if response.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("expected allow header, got %q", response.Header().Get("Allow"))
	}
}

func TestServeHTTPLeavesUnknownAssetPathToLookupFlow(t *testing.T) {
	handler := newTestHandler(&fakeRedisStore{})
	request := httptest.NewRequest(http.MethodGet, "/asset/not-exist.txt", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", response.Code)
	}
}

func TestServeHTTPReturnsTopicHomeWithTopicCacheHeader(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:anime": {value: `{"type":"topic","content":"<html><body>Anime</body></html>","title":"Anime Archive"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/anime", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if response.Header().Get("Cache-Control") != topicCacheControl {
		t.Fatalf("expected topic cache header %q, got %q", topicCacheControl, response.Header().Get("Cache-Control"))
	}
	if !strings.Contains(response.Body.String(), "Anime") {
		t.Fatalf("expected topic body, got %q", response.Body.String())
	}
}

func TestServeHTTPReturnsHTMLWithDefaultPublicCacheHeader(t *testing.T) {
	store := &fakeRedisStore{
		getResults: map[string]fakeStringResult{
			"surl:note": {value: `{"type":"html","content":"<html><body>Note</body></html>","title":"Note"}`},
		},
	}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodGet, "/note", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}
	if response.Header().Get("Cache-Control") != publicCacheControl {
		t.Fatalf("expected default cache header %q, got %q", publicCacheControl, response.Header().Get("Cache-Control"))
	}
}

func TestHandleJSONCreateRejectsReservedAssetPath(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"hello","path":"asset/md-base-7f7c1c5a.css","type":"text"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleJSONCreate(response, request, false)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Code != "invalid_request" {
		t.Fatalf("expected invalid_request, got %+v", body)
	}
}

func TestHandleDeleteRejectsReservedAssetPath(t *testing.T) {
	store := &fakeRedisStore{}
	handler := newTestHandler(store)
	request := httptest.NewRequest(http.MethodDelete, "/", strings.NewReader(`{"path":"asset/md-base-7f7c1c5a.css","type":"text"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	handler.handleDelete(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", response.Code)
	}
	body := decodeErrorPayload(t, response)
	if body.Code != "invalid_request" {
		t.Fatalf("expected invalid_request, got %+v", body)
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
	stored := storage.ParseStoredValue(store.lastSetValue)
	if stored.Type != "file" || stored.Content != "post/default/uploaded.txt" || stored.Title != "Attachment" {
		t.Fatalf("unexpected stored value: %+v", stored)
	}
	assertRFC3339Value(t, stored.Created)
	var body CreateResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Title != "Attachment" {
		t.Fatalf("expected create title Attachment, got %+v", body)
	}
	assertRFC3339Value(t, body.Created)
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

func assertRFC3339Value(t *testing.T, value string) {
	t.Helper()
	if value == "" {
		t.Fatalf("expected non-empty created value")
	}
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		t.Fatalf("expected RFC3339 created value, got %q (%v)", value, err)
	}
}

func newMultipartUploadRequest(t *testing.T, method string, fields map[string]string, filename string, content string) *http.Request {
	return newMultipartUploadRequestWithFileContentType(t, method, fields, filename, content, "")
}

func newMultipartUploadRequestWithFileContentType(t *testing.T, method string, fields map[string]string, filename string, content string, fileContentType string) *http.Request {
	t.Helper()

	body := &strings.Builder{}
	writer := multipart.NewWriter(body)
	partHeader := textproto.MIMEHeader{}
	partHeader.Set("Content-Disposition", `form-data; name="file"; filename="`+filename+`"`)
	if fileContentType != "" {
		partHeader.Set("Content-Type", fileContentType)
	}
	fileWriter, err := writer.CreatePart(partHeader)
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
