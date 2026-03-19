package utils

import (
	"encoding/json"
	"io"
	"net/http"
)

// JSON writes JSON response with trailing newline.
func JSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(payload)
}

// Error writes a unified error response payload.
type ErrorPayload struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Hint    any    `json:"hint,omitempty"`
	Details any    `json:"details,omitempty"`
}

func Error(w http.ResponseWriter, status int, code, message string, hint any, details any) {
	JSON(w, status, ErrorPayload{
		Error:   message,
		Code:    code,
		Hint:    hint,
		Details: details,
	})
}

// Text writes plain text response.
func Text(w http.ResponseWriter, status int, text string, cache bool) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if cache && w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=86400")
	}
	w.WriteHeader(status)
	_, _ = io.WriteString(w, text)
	_, _ = io.WriteString(w, "\n")
}

// HTML writes HTML response.
func HTML(w http.ResponseWriter, status int, html string, cache bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if cache && w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=86400")
	}
	w.WriteHeader(status)
	_, _ = io.WriteString(w, html)
}

// Redirect sends 302 redirect.
func Redirect(w http.ResponseWriter, r *http.Request, url string, cache bool) {
	if cache && w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=86400")
	}
	http.Redirect(w, r, url, http.StatusFound)
}

// Binary writes binary content.
func Binary(w http.ResponseWriter, status int, body []byte, contentType string, contentLength int64, cache bool) {
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if contentLength > 0 {
		w.Header().Set("Content-Length", itoa64(contentLength))
	}
	if cache && w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "public, max-age=86400, s-maxage=86400")
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [32]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// Itoa64 converts int64 to decimal string.
func Itoa64(v int64) string {
	return itoa64(v)
}
