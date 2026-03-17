package httpapi

import (
	"context"
	"crypto/rand"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"post-go/internal/storage"
)

const (
	topicHomeManagedError = "topic home must be managed with `type=topic`"
	maxTTLMinutes         = int64(525600)
	maxTTLSeconds         = int64(31536000)
)

func randomPath() string {
	chars := "23456789abcdefghjkmnpqrstuvwxyz"
	b := make([]byte, 5)
	for i := 0; i < 5; i++ {
		idx, err := randIndex(len(chars))
		if err != nil {
			idx = 0
		}
		b[i] = chars[idx]
	}
	return string(b)
}

func randIndex(max int) (int, error) {
	if max <= 0 {
		return 0, errors.New("invalid max")
	}
	var b [1]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 0, err
	}
	return int(b[0]) % max, nil
}

func isURL(s string) bool {
	_, err := parseURLValue(s)
	return err == nil
}

func normalizeURLContent(content string) (string, error) {
	trimmedContent := strings.TrimSpace(content)
	if trimmedContent == "" {
		return "", errors.New("`url` must not be empty")
	}
	if _, err := parseURLValue(trimmedContent); err != nil {
		return "", errors.New("invalid url value: scheme is required")
	}
	return trimmedContent, nil
}

func parseURLValue(raw string) (*url.URL, error) {
	parsedURL, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if parsedURL.Scheme == "" {
		return nil, errors.New("missing scheme")
	}
	return parsedURL, nil
}

func parseInt64(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("invalid")
		}
		n = n*10 + int64(r-'0')
	}
	return n, nil
}

func validateTTLMinutes(ttlMinutes int64) (int64, error) {
	if ttlMinutes < 0 {
		return 0, errors.New("`ttl` must be a natural number")
	}
	if ttlMinutes > maxTTLMinutes {
		return 0, errors.New("`ttl` must be between 0 and 525600 minutes")
	}
	return ttlMinutes, nil
}

func parseTTLValue(ttlValue any) (int64, bool, error) {
	if ttlValue == nil {
		return 0, false, nil
	}
	switch value := ttlValue.(type) {
	case int64:
		ttlMinutes, err := validateTTLMinutes(value)
		return ttlMinutes, true, err
	case int:
		ttlMinutes, err := validateTTLMinutes(int64(value))
		return ttlMinutes, true, err
	default:
		ttlMinutes, ok := storage.MustInt(map[string]any{"ttl": ttlValue}, "ttl")
		if !ok {
			return 0, true, errors.New("`ttl` must be a natural number")
		}
		ttlMinutes, err := validateTTLMinutes(ttlMinutes)
		return ttlMinutes, true, err
	}
}

func parseTTLFormValue(ttlValue string) (int64, bool, error) {
	if ttlValue == "" {
		return 0, false, nil
	}
	ttlMinutes, err := parseInt64(ttlValue)
	if err != nil {
		return 0, true, errors.New("`ttl` must be a natural number")
	}
	ttlMinutes, err = validateTTLMinutes(ttlMinutes)
	return ttlMinutes, true, err
}

func ttlSecondsFromMinutes(ttlMinutes int64) int64 {
	if ttlMinutes <= 0 {
		return 0
	}
	ttlSeconds := ttlMinutes * 60
	if ttlSeconds > maxTTLSeconds {
		return maxTTLSeconds
	}
	return ttlSeconds
}

// setStoredValueWithTTL keeps omitted TTL and ttl=0 on the same persistent path.
func setStoredValueWithTTL(ctx context.Context, rdb redisStore, key, storedValue string, ttlMinutes int64, ttlProvided bool) (any, error) {
	if !ttlProvided || ttlMinutes == 0 {
		if err := rdb.Set(ctx, key, storedValue, 0).Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err := rdb.SetEx(ctx, key, storedValue, time.Duration(ttlMinutes)*time.Minute).Err(); err != nil {
		return nil, err
	}
	return ttlMinutes, nil
}

func restoreStoredValueWithTTL(ctx context.Context, rdb redisStore, key, storedValue string, previousTTL time.Duration) error {
	if previousTTL > 0 {
		return rdb.SetEx(ctx, key, storedValue, previousTTL).Err()
	}
	return rdb.Set(ctx, key, storedValue, 0).Err()
}

func isExportRequest(r *http.Request) bool {
	return strings.ToLower(r.Header.Get("x-export")) == "true"
}

func responseContent(typ, content string, isExport bool) string {
	if isExport {
		return content
	}
	return storage.PreviewContent(typ, content)
}

func buildItemResponse(domain, path string, storedValue storage.StoredValue, ttl *int64, isExport bool) ItemResponse {
	return ItemResponse{
		SURL:    domain + "/" + path,
		Path:    path,
		Type:    storedValue.Type,
		Title:   storedValue.Title,
		TTL:     ttl,
		Content: responseContent(storedValue.Type, storedValue.Content, isExport),
	}
}

func hasEmptyPathSegment(path string) bool {
	if path == "" {
		return false
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "" {
			return true
		}
	}
	return false
}

func ttlMinutesFromDuration(ttlDuration time.Duration) *int64 {
	if ttlDuration <= 0 {
		return nil
	}
	ttlMinutes := int64(math.Ceil(ttlDuration.Minutes()))
	if ttlMinutes < 1 {
		ttlMinutes = 1
	}
	return &ttlMinutes
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	buf := make([]byte, 0, 12)
	for v > 0 {
		buf = append(buf, byte('0'+v%10))
		v /= 10
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
