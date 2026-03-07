package storage

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

const (
	LinksPrefix   = "surl:"
	PreviewLength = 15
)

// BuildStoredValue builds "type:content" string.
func BuildStoredValue(typ, content string) string {
	return typ + ":" + content
}

// ParseStoredValue parses stored value into type and content.
func ParseStoredValue(stored string) (typ, content string) {
	if strings.HasPrefix(stored, "url:") {
		return "url", strings.TrimPrefix(stored, "url:")
	}
	if strings.HasPrefix(stored, "html:") {
		return "html", strings.TrimPrefix(stored, "html:")
	}
	if strings.HasPrefix(stored, "file:") {
		return "file", strings.TrimPrefix(stored, "file:")
	}
	if strings.HasPrefix(stored, "text:") {
		return "text", strings.TrimPrefix(stored, "text:")
	}
	return "text", stored
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
	if err := json.Unmarshal(body, &out); err != nil {
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
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	default:
		return 0, false
	}
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
