package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	LinksPrefix   = "surl:"
	PreviewLength = 15
)

// StoredValue is the JSON payload persisted in Redis for a content item.
type StoredValue struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Title   string `json:"title,omitempty"`
	Created string `json:"created,omitempty"`
}

var defaultCreatedLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// BuildStoredValue marshals a stored value to JSON.
func BuildStoredValue(value StoredValue) string {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(value)
	if err != nil {
		panic(err)
	}
	data := buf.Bytes()
	if len(data) > 0 && data[len(data)-1] == '\n' {
		data = data[:len(data)-1]
	}
	return string(data)
}

// ParseStoredValue parses a JSON stored value.
func ParseStoredValue(stored string) StoredValue {
	var value StoredValue
	if err := json.Unmarshal([]byte(stored), &value); err != nil {
		return StoredValue{Type: "text", Content: stored}
	}
	if value.Type == "" {
		value.Type = "text"
	}
	return value
}

// PreviewContent returns a preview string for list/detail responses.
func PreviewContent(typ, content string) string {
	if typ == "url" || typ == "file" {
		return content
	}
	if len(content) > PreviewLength {
		return content[:PreviewLength] + "..."
	}
	return content
}

// GetDomain builds protocol + host from forwarded headers.
func GetDomain(r *http.Request) string {
	proto := r.Header.Get("x-forwarded-proto")
	if proto == "" {
		proto = "http"
	}
	host := r.Header.Get("x-forwarded-host")
	if host == "" {
		host = r.Host
	}
	return proto + "://" + host
}

// ParseJSONBody reads and parses JSON body.
func ParseJSONBody(r *http.Request) (map[string]any, error) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return map[string]any{}, nil
	}
	var out map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// MustString reads a string field from map.
func MustString(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// MustInt reads a number field from map (supports json.Number).
func MustInt(m map[string]any, key string) (int64, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return 0, false
		}
		return i, true
	case int64:
		return t, true
	case int:
		return int64(t), true
	default:
		return 0, false
	}
}

// NormalizePath trims leading and trailing slashes, except slash-only inputs map to "/".
func NormalizePath(path string) string {
	if path == "" {
		return ""
	}
	if strings.Trim(path, "/") == "" {
		return "/"
	}
	return strings.Trim(path, "/")
}

// ValidatePath checks allowed characters and length.
func ValidatePath(path string) error {
	if len(path) < 1 || len(path) > 99 {
		return errors.New("path must be 1-99 characters")
	}
	for _, r := range path {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.' || r == '/' || r == '(' || r == ')':
		default:
			return errors.New("path can only contain: a-z A-Z 0-9 - _ . / ( )")
		}
	}
	return nil
}

// ParseCreatedTime parses accepted created input formats.
func ParseCreatedTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, errors.New("empty created")
	}
	layoutsWithTimezone := []string{
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layoutsWithTimezone {
		parsed, err := time.Parse(layout, raw)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	layoutsWithoutTimezone := []string{
		"2006-01-02 15:04:05",
		"2006-01-02",
		"2006.01.02",
		"2006/01/02",
	}
	for _, layout := range layoutsWithoutTimezone {
		parsed, err := time.ParseInLocation(layout, raw, defaultCreatedLocation)
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, errors.New("invalid created format")
}

// NormalizeCreatedTime converts supported created input into UTC RFC3339.
func NormalizeCreatedTime(raw string) (string, error) {
	parsed, err := ParseCreatedTime(raw)
	if err != nil {
		return "", err
	}
	return parsed.Format(time.RFC3339), nil
}
