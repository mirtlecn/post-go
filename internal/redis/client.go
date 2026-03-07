package redisx

import (
	"context"
	"errors"
	"sync"

	"github.com/redis/go-redis/v9"
)

var (
	client *redis.Client
	once   sync.Once
)

// GetClient returns a singleton Redis client.
func GetClient(url string) (*redis.Client, error) {
	if url == "" {
		return nil, errors.New("LINKS_REDIS_URL environment variable is not set")
	}
	var err error
	once.Do(func() {
		opt, e := redis.ParseURL(url)
		if e != nil {
			err = e
			return
		}
		client = redis.NewClient(opt)
		if e := client.Ping(context.Background()).Err(); e != nil {
			err = e
			return
		}
	})
	return client, err
}
