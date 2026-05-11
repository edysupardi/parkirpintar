package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Config struct {
	Host       string
	Port       int
	Password   string
	DB         int
	MaxRetries int
}

func (c Config) addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c Config) validate() error {
	if c.Host == "" {
		return fmt.Errorf("redis host is required")
	}
	if c.Port == 0 {
		return fmt.Errorf("redis port is required")
	}
	return nil
}

func (c Config) ExportValidate() error { return c.validate() }
func (c Config) ExportAddr() string    { return c.addr() }

type Client struct {
	rdb *redis.Client
}

func New(cfg Config) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:       cfg.addr(),
		Password:   cfg.Password,
		DB:         cfg.DB,
		MaxRetries: cfg.MaxRetries,
	})

	return &Client{rdb: rdb}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.rdb.Ping(ctx).Err()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.rdb.Get(ctx, key).Result()
}

func (c *Client) Set(ctx context.Context, key string, value any, expiry time.Duration) error {
	return c.rdb.Set(ctx, key, value, expiry).Err()
}

// SetNX sets key only if it does not exist. Returns true if the key was set.
func (c *Client) SetNX(ctx context.Context, key string, value any, expiry time.Duration) (bool, error) {
	return c.rdb.SetNX(ctx, key, value, expiry).Result()
}

func (c *Client) Del(ctx context.Context, keys ...string) error {
	return c.rdb.Del(ctx, keys...).Err()
}

func (c *Client) Close() error {
	return c.rdb.Close()
}
