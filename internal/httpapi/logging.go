package httpapi

import (
	"log"
	"net/http"
	"time"
)

// requestLogger provides a simple leveled logging wrapper.
type requestLogger struct{}

var debugEnabled bool

func setDebugEnabled(v bool) {
	debugEnabled = v
}

func (requestLogger) Infof(format string, args ...any) {
	log.Printf("[INFO] "+format, args...)
}

func (requestLogger) Debugf(format string, args ...any) {
	if !debugEnabled {
		return
	}
	log.Printf("[DEBUG] "+format, args...)
}

func (requestLogger) Warnf(format string, args ...any) {
	log.Printf("[WARN] "+format, args...)
}

func (requestLogger) Errorf(format string, args ...any) {
	log.Printf("[ERROR] "+format, args...)
}

// responseRecorder captures status and bytes written.
type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func withRecorder(w http.ResponseWriter) *responseRecorder {
	return &responseRecorder{ResponseWriter: w}
}

func logRequestDone(l requestLogger, r *http.Request, rec *responseRecorder, started time.Time) {
	status := rec.status
	if status == 0 {
		status = http.StatusOK
	}
	l.Infof("response: %s %s -> %d (%d bytes) in %s",
		r.Method, r.URL.Path, status, rec.bytes, time.Since(started).String())
}
