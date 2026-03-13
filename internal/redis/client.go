package redisx

import (
	"context"
	"errors"
	"sync"

	"github.com/redis/go-redis/v9"
)

var (
	clientFactory = redis.NewClient
	pingClient    = pingRedisClient
	clientStore   = newClientCache()
)

type clientCache struct {
	mu      sync.RWMutex
	clients map[string]*redis.Client
}

func newClientCache() *clientCache {
	return &clientCache{
		clients: make(map[string]*redis.Client),
	}
}

func (c *clientCache) get(url string) (*redis.Client, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	client, ok := c.clients[url]
	return client, ok
}

func (c *clientCache) set(url string, client *redis.Client) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clients[url] = client
}

// GetClient returns a cached Redis client per URL.
func GetClient(url string) (*redis.Client, error) {
	if url == "" {
		return nil, errors.New("LINKS_REDIS_URL environment variable is not set")
	}
	if client, ok := clientStore.get(url); ok {
		return client, nil
	}

	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}

	client := clientFactory(opt)
	if err := pingClient(context.Background(), client); err != nil {
		return nil, err
	}
	clientStore.set(url, client)
	return client, nil
}

func pingRedisClient(ctx context.Context, client *redis.Client) error {
	return client.Ping(ctx).Err()
}
