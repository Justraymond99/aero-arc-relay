package redisconn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrDisabled = errors.New("redis disabled (REDIS_ADDR not set)")

// Client wraps a go-redis client and can be shared across components.
type Client struct {
	rdb *redis.Client
}

// Close closes the underlying Redis client.
func (c *Client) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

// Ping checks connectivity to Redis.
func (c *Client) Ping(ctx context.Context) error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Ping(ctx).Err()
}

// NewClientFromEnv creates a Redis client from environment variables.
//
// Environment variables:
//   - REDIS_ADDR      (required to enable Redis, e.g. "localhost:6379")
//   - REDIS_PASSWORD  (optional)
//   - REDIS_DB        (optional, integer DB index; defaults to 0)
//
// Behaviour:
//   - If REDIS_ADDR is not set, Redis is treated as disabled and ErrDisabled is returned.
//   - If ping fails, the client is returned alongside an error; callers may continue
//     running and implement retry/backoff logic as needed.
func NewClientFromEnv(ctx context.Context) (*Client, error) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		return nil, ErrDisabled
	}

	password := os.Getenv("REDIS_PASSWORD")
	db := 0
	if dbStr := os.Getenv("REDIS_DB"); dbStr != "" {
		// Ignore parse errors and keep db=0; this avoids crashing on bad input.
		if parsed, err := parseDB(dbStr); err == nil {
			db = parsed
		} else {
			slog.LogAttrs(ctx, slog.LevelWarn, "Invalid REDIS_DB value, defaulting to 0", slog.String("error", err.Error()))
		}
	}

	opts := &redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	}

	rdb := redis.NewClient(opts)

	// Perform a short ping on startup to surface connectivity issues without
	// crashing the relay.
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client := &Client{rdb: rdb}
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return client, fmt.Errorf("redis ping failed: %w", err)
	}
	return client, nil
}

// parseDB converts a REDIS_DB string into an integer index.
func parseDB(value string) (int, error) {
	// Small, local parse to avoid pulling in strconv here unnecessarily.
	var n int
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("non-digit character %q in DB index", ch)
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}
