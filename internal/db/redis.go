package db

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis wraps a redis.Client. All keys written by WuNest should use the
// "nest:" prefix to avoid colliding with WuApi's Redis usage on the shared
// instance.
type Redis struct {
	*redis.Client
}

// NewRedis connects to Redis using the given URL and runs a ping.
func NewRedis(ctx context.Context, url string) (*Redis, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return &Redis{Client: client}, nil
}
