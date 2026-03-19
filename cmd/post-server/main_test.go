package main

import (
	"errors"
	"net/http"
	"testing"
)

func TestServeReturnsListenError(t *testing.T) {
	expectedErr := errors.New("listen tcp :3051: bind: address already in use")

	err := serve(
		":3051",
		http.NotFoundHandler(),
		"3051",
		"redis://127.0.0.1:6379/2",
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
