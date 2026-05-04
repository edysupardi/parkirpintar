package idempotency

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const DefaultTTL = 24 * time.Hour

type Store struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

// Check returns the cached response for the given key, or ("", false, nil) on miss.
func (s *Store) Check(ctx context.Context, key string) (string, bool, error) {
	val, err := s.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

// Save stores the response under the given key with the specified TTL.
func (s *Store) Save(ctx context.Context, key, response string, ttl time.Duration) error {
	return s.rdb.Set(ctx, key, response, ttl).Err()
}
