package testutil

import (
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type TestRedis struct {
	Client *redis.Client
	Server *miniredis.Miniredis
}

func NewTestRedis() (*TestRedis, error) {
	s, err := miniredis.Run()
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(&redis.Options{
		Addr: s.Addr(),
	})

	return &TestRedis{Client: client, Server: s}, nil
}

func (r *TestRedis) Close() {
	r.Client.Close()
	r.Server.Close()
}

func (r *TestRedis) FastForward(d time.Duration) {
	r.Server.FastForward(d)
}

func (r *TestRedis) FlushAll() {
	r.Server.FlushAll()
}
