package httpapi

import (
	"crypto/rand"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"post-go/internal/storage"
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

func hasKey(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
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
