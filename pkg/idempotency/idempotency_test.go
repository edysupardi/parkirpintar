package idempotency_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) (*idempotency.Store, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return idempotency.New(rdb), mr
}

func TestStore_Check_Miss(t *testing.T) {
	store, _ := newTestStore(t)
	val, hit, err := store.Check(context.Background(), "idempotency:unknown-key")
	assert.NoError(t, err)
	assert.False(t, hit)
	assert.Empty(t, val)
}

func TestStore_Save_And_Check_Hit(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	err := store.Save(ctx, "idempotency:key-1", `{"status":"ok"}`, idempotency.DefaultTTL)
	require.NoError(t, err)

	val, hit, err := store.Check(ctx, "idempotency:key-1")
	assert.NoError(t, err)
	assert.True(t, hit)
	assert.Equal(t, `{"status":"ok"}`, val)
}

func TestStore_Check_Expired(t *testing.T) {
	store, mr := newTestStore(t)
	ctx := context.Background()

	err := store.Save(ctx, "idempotency:key-2", `{"status":"ok"}`, 1*time.Second)
	require.NoError(t, err)

	mr.FastForward(2 * time.Second)

	_, hit, err := store.Check(ctx, "idempotency:key-2")
	assert.NoError(t, err)
	assert.False(t, hit)
}
