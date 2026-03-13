package redisx

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
)

func TestGetClientRetriesAfterInitialFailure(t *testing.T) {
	resetRedisClientState()

	attempts := 0
	clientFactory = func(opt *redis.Options) *redis.Client {
		return redis.NewClient(opt)
	}
	pingClient = func(ctx context.Context, client *redis.Client) error {
		attempts++
		if attempts == 1 {
			return errors.New("dial failed")
		}
		return nil
	}
	t.Cleanup(resetRedisClientState)

	if _, err := GetClient("redis://127.0.0.1:6379/0"); err == nil {
		t.Fatalf("expected first call to fail")
	}

	client, err := GetClient("redis://127.0.0.1:6379/0")
	if err != nil {
		t.Fatalf("expected second call to retry and succeed, got %v", err)
	}
	if client == nil {
		t.Fatalf("expected client after retry")
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestGetClientCachesPerURL(t *testing.T) {
	resetRedisClientState()

	attemptsByURL := map[string]int{}
	clientFactory = func(opt *redis.Options) *redis.Client {
		return redis.NewClient(opt)
	}
	pingClient = func(ctx context.Context, client *redis.Client) error {
		attemptsByURL[client.Options().Addr]++
		return nil
	}
	t.Cleanup(resetRedisClientState)

	firstClient, err := GetClient("redis://127.0.0.1:6379/0")
	if err != nil {
		t.Fatalf("expected first URL to succeed, got %v", err)
	}
	secondClient, err := GetClient("redis://127.0.0.1:6380/0")
	if err != nil {
		t.Fatalf("expected second URL to succeed, got %v", err)
	}
	cachedClient, err := GetClient("redis://127.0.0.1:6379/0")
	if err != nil {
		t.Fatalf("expected cached URL to succeed, got %v", err)
	}

	if firstClient != cachedClient {
		t.Fatalf("expected identical cached client for same URL")
	}
	if firstClient == secondClient {
		t.Fatalf("expected different clients for different URLs")
	}
	if attemptsByURL["127.0.0.1:6379"] != 1 {
		t.Fatalf("expected first URL to initialize once, got %d", attemptsByURL["127.0.0.1:6379"])
	}
	if attemptsByURL["127.0.0.1:6380"] != 1 {
		t.Fatalf("expected second URL to initialize once, got %d", attemptsByURL["127.0.0.1:6380"])
	}
}

func resetRedisClientState() {
	clientFactory = redis.NewClient
	pingClient = pingRedisClient
	clientStore = newClientCache()
}
