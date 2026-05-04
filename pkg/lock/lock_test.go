package lock_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/edysupardi/parkirpintar/pkg/lock"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLocker(t *testing.T) (*lock.Locker, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return lock.New(rdb), mr
}

func TestLocker_Acquire_Success(t *testing.T) {
	locker, _ := newTestLocker(t)
	ok, err := locker.Acquire(context.Background(), "spot:1:lock", "res-abc", time.Hour)
	assert.NoError(t, err)
	assert.True(t, ok)
}

func TestLocker_Acquire_AlreadyHeld(t *testing.T) {
	locker, _ := newTestLocker(t)
	ctx := context.Background()

	_, _ = locker.Acquire(ctx, "spot:1:lock", "res-abc", time.Hour)
	ok, err := locker.Acquire(ctx, "spot:1:lock", "res-xyz", time.Hour)
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestLocker_Release_Success(t *testing.T) {
	locker, mr := newTestLocker(t)
	ctx := context.Background()

	_, _ = locker.Acquire(ctx, "spot:1:lock", "res-abc", time.Hour)
	err := locker.Release(ctx, "spot:1:lock", "res-abc")
	assert.NoError(t, err)
	assert.Equal(t, "", mr.HGet("spot:1:lock", ""))
}

func TestLocker_Release_WrongValue(t *testing.T) {
	locker, mr := newTestLocker(t)
	ctx := context.Background()

	_, _ = locker.Acquire(ctx, "spot:1:lock", "res-abc", time.Hour)
	err := locker.Release(ctx, "spot:1:lock", "res-xyz") // wrong value
	assert.NoError(t, err)
	// lock should still exist
	val, _ := mr.Get("spot:1:lock")
	assert.Equal(t, "res-abc", val)
}

func TestLocker_Acquire_AfterRelease(t *testing.T) {
	locker, _ := newTestLocker(t)
	ctx := context.Background()

	_, _ = locker.Acquire(ctx, "spot:1:lock", "res-abc", time.Hour)
	_ = locker.Release(ctx, "spot:1:lock", "res-abc")

	ok, err := locker.Acquire(ctx, "spot:1:lock", "res-xyz", time.Hour)
	assert.NoError(t, err)
	assert.True(t, ok)
}
