package main

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"strings"
	"testing"
)

func TestServeReturnsListenError(t *testing.T) {
	expectedErr := errors.New("listen tcp :3051: bind: address already in use")

	err := serve(
		":3051",
		http.NotFoundHandler(),
		"3051",
		func(addr string, handler http.Handler) error {
			if addr != ":3051" {
				t.Fatalf("expected addr to be passed through, got %q", addr)
			}
			if handler == nil {
				t.Fatal("expected handler to be passed through")
			}
			return expectedErr
		},
	)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected listen error to be returned, got %v", err)
	}
}

func TestServeDoesNotLogRedisURL(t *testing.T) {
	var logBuffer bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logBuffer)
	defer log.SetOutput(previousWriter)

	err := serve(
		":3051",
		http.NotFoundHandler(),
		"3051",
		func(addr string, handler http.Handler) error {
			return nil
		},
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if strings.Contains(logBuffer.String(), "LINKS_REDIS_URL") || strings.Contains(logBuffer.String(), "redis://") {
		t.Fatalf("expected redis url to be absent from logs, got %q", logBuffer.String())
	}
}
