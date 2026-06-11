package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type fakeFileCacheStore struct {
	values     map[string][]byte
	setExKeys  []string
	setExTTLs  []time.Duration
	unlinkKeys [][]string
	unlinkErr  error
	delKeys    [][]string
	delErr     error
}

func (f *fakeFileCacheStore) Get(ctx context.Context, key string) *redis.StringCmd {
	value, ok := f.values[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(string(value), nil)
}

func (f *fakeFileCacheStore) SetEx(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	f.setExKeys = append(f.setExKeys, key)
	f.setExTTLs = append(f.setExTTLs, expiration)
	if f.values == nil {
		f.values = map[string][]byte{}
	}
	switch typed := value.(type) {
	case []byte:
		f.values[key] = append([]byte(nil), typed...)
	case string:
		f.values[key] = []byte(typed)
	default:
		return redis.NewStatusResult("", errors.New("unexpected value type"))
	}
	return redis.NewStatusResult("OK", nil)
}

func (f *fakeFileCacheStore) Unlink(ctx context.Context, keys ...string) *redis.IntCmd {
	f.unlinkKeys = append(f.unlinkKeys, append([]string(nil), keys...))
	return redis.NewIntResult(int64(len(keys)), f.unlinkErr)
}

func (f *fakeFileCacheStore) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	f.delKeys = append(f.delKeys, append([]string(nil), keys...))
	return redis.NewIntResult(int64(len(keys)), f.delErr)
}

func TestSetFileCacheStoresSingleBinaryKey(t *testing.T) {
	store := &fakeFileCacheStore{}
	item := &FileCacheItem{
		Buffer:        []byte{0, 1, 2, 'h', 'i'},
		ContentType:   "text/html",
		ContentLength: 5,
	}

	if err := SetFileCache(context.Background(), store, "asset.bin", item); err != nil {
		t.Fatalf("expected set file cache to succeed, got %v", err)
	}

	if len(store.setExKeys) != 1 || store.setExKeys[0] != "cache:file:asset.bin" {
		t.Fatalf("expected single file cache key, got %+v", store.setExKeys)
	}
	if len(store.setExTTLs) != 1 || store.setExTTLs[0] != fileCacheTTL {
		t.Fatalf("expected file cache ttl %v, got %+v", fileCacheTTL, store.setExTTLs)
	}
	payload := store.values[fileCacheKey("asset.bin")]
	meta, body := decodeFileCachePayloadForTest(t, payload)
	if meta.ContentType != "text/html" || meta.ContentLength != 5 {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	if !bytes.Equal(body, item.Buffer) {
		t.Fatalf("expected raw body bytes %v, got %v", item.Buffer, body)
	}
	for _, oldField := range []string{"contentType", "contentLength", "encoding", "checksum"} {
		if bytes.Contains(payload, []byte(oldField)) {
			t.Fatalf("expected payload to omit old field %q, got %q", oldField, payload)
		}
	}
}

func TestSetFileCacheDefaultsEmptyContentType(t *testing.T) {
	store := &fakeFileCacheStore{}
	item := &FileCacheItem{
		Buffer:        []byte("body"),
		ContentLength: 4,
	}

	if err := SetFileCache(context.Background(), store, "asset.bin", item); err != nil {
		t.Fatalf("expected set file cache to succeed, got %v", err)
	}

	meta, _ := decodeFileCachePayloadForTest(t, store.values[fileCacheKey("asset.bin")])
	if meta.ContentType != "application/octet-stream" {
		t.Fatalf("expected default content type, got %+v", meta)
	}
}

func TestGetFileCacheReadsSingleBinaryKey(t *testing.T) {
	store := &fakeFileCacheStore{}
	item := &FileCacheItem{
		Buffer:        []byte{0, 1, 2, 'h', 'i'},
		ContentType:   "application/octet-stream",
		ContentLength: 5,
	}
	if err := SetFileCache(context.Background(), store, "asset.bin", item); err != nil {
		t.Fatalf("expected set file cache to succeed, got %v", err)
	}

	got, err := GetFileCache(context.Background(), store, "asset.bin")
	if err != nil {
		t.Fatalf("expected get file cache to succeed, got %v", err)
	}
	if got == nil {
		t.Fatal("expected cached item, got nil")
	}
	if got.ContentType != item.ContentType || got.ContentLength != item.ContentLength || !bytes.Equal(got.Buffer, item.Buffer) {
		t.Fatalf("unexpected cached item: %+v", got)
	}
}

func TestGetFileCacheUsesStoredMetadataContentLength(t *testing.T) {
	store := &fakeFileCacheStore{
		values: map[string][]byte{
			fileCacheKey("asset.bin"): encodeFileCachePayloadForTest(fileCacheMetadata{ContentType: "text/plain", ContentLength: 99}, []byte("hi")),
		},
	}

	got, err := GetFileCache(context.Background(), store, "asset.bin")
	if err != nil {
		t.Fatalf("expected get file cache to succeed, got %v", err)
	}
	if got == nil || got.ContentLength != 99 || !bytes.Equal(got.Buffer, []byte("hi")) {
		t.Fatalf("expected metadata content length to be preserved, got %+v", got)
	}
}

func TestGetFileCacheTreatsMalformedPayloadAsMiss(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "short header", payload: []byte{0, 1, 2}},
		{name: "metadata length overflow", payload: fileCachePayloadWithLengthForTest(99, nil, nil)},
		{name: "bad json", payload: fileCachePayloadWithLengthForTest(1, []byte("{"), nil)},
		{name: "missing content type", payload: rawFileCachePayloadForTest([]byte(`{"cl":4}`), []byte("body"))},
		{name: "string content length", payload: rawFileCachePayloadForTest([]byte(`{"ct":"text/plain","cl":"4"}`), []byte("body"))},
		{name: "negative content length", payload: encodeFileCachePayloadForTest(fileCacheMetadata{ContentType: "text/plain", ContentLength: -1}, []byte("body"))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeFileCacheStore{
				values: map[string][]byte{fileCacheKey("asset.bin"): test.payload},
			}

			got, err := GetFileCache(context.Background(), store, "asset.bin")
			if err != nil {
				t.Fatalf("expected malformed cache to be ignored, got %v", err)
			}
			if got != nil {
				t.Fatalf("expected cache miss, got %+v", got)
			}
		})
	}
}

func TestClearFileCacheUsesSingleKey(t *testing.T) {
	store := &fakeFileCacheStore{}

	if err := ClearFileCache(context.Background(), store, "asset.bin"); err != nil {
		t.Fatalf("expected clear file cache to succeed, got %v", err)
	}

	if len(store.unlinkKeys) != 1 || len(store.unlinkKeys[0]) != 1 || store.unlinkKeys[0][0] != "cache:file:asset.bin" {
		t.Fatalf("expected unlink single cache key, got %+v", store.unlinkKeys)
	}
	if len(store.delKeys) != 0 {
		t.Fatalf("expected no del fallback, got %+v", store.delKeys)
	}
}

func TestClearFileCacheFallsBackToDelSingleKey(t *testing.T) {
	store := &fakeFileCacheStore{unlinkErr: errors.New("unlink failed")}

	if err := ClearFileCache(context.Background(), store, "asset.bin"); err != nil {
		t.Fatalf("expected clear file cache fallback to succeed, got %v", err)
	}

	if len(store.delKeys) != 1 || len(store.delKeys[0]) != 1 || store.delKeys[0][0] != "cache:file:asset.bin" {
		t.Fatalf("expected del single cache key, got %+v", store.delKeys)
	}
}

func decodeFileCachePayloadForTest(t *testing.T, payload []byte) (fileCacheMetadata, []byte) {
	t.Helper()

	if len(payload) < 4 {
		t.Fatalf("payload too short: %v", payload)
	}
	metaLength := binary.BigEndian.Uint32(payload[:4])
	if metaLength > uint32(len(payload)-4) {
		t.Fatalf("invalid metadata length %d for payload %v", metaLength, payload)
	}
	bodyStart := 4 + int(metaLength)
	var meta fileCacheMetadata
	if err := json.Unmarshal(payload[4:bodyStart], &meta); err != nil {
		t.Fatalf("failed to decode metadata: %v", err)
	}
	return meta, payload[bodyStart:]
}

func encodeFileCachePayloadForTest(meta fileCacheMetadata, body []byte) []byte {
	metaBytes, _ := json.Marshal(meta)
	return fileCachePayloadWithLengthForTest(uint32(len(metaBytes)), metaBytes, body)
}

func rawFileCachePayloadForTest(metaBytes, body []byte) []byte {
	return fileCachePayloadWithLengthForTest(uint32(len(metaBytes)), metaBytes, body)
}

func fileCachePayloadWithLengthForTest(metaLength uint32, metaBytes, body []byte) []byte {
	payload := make([]byte, 4+len(metaBytes)+len(body))
	binary.BigEndian.PutUint32(payload[:4], metaLength)
	copy(payload[4:], metaBytes)
	copy(payload[4+len(metaBytes):], body)
	return payload
}
