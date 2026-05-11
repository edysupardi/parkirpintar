package lock

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// DB is the Redis interface required by this package.
type DB interface {
	SetNX(ctx context.Context, key string, value any, expiry time.Duration) (bool, error)
	Get(ctx context.Context, key string) (string, error)
}

var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("DEL", KEYS[1])
else
    return 0
end
`)

type Locker struct {
	rdb *redis.Client
}

func New(rdb *redis.Client) *Locker {
	return &Locker{rdb: rdb}
}

// Acquire tries to set the lock. Returns true if acquired, false if already held.
func (l *Locker) Acquire(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	return l.rdb.SetNX(ctx, key, value, ttl).Result()
}

// Release deletes the lock only if the value matches — prevents releasing another holder's lock.
func (l *Locker) Release(ctx context.Context, key, value string) error {
	return releaseScript.Run(ctx, l.rdb, []string{key}, value).Err()
}
