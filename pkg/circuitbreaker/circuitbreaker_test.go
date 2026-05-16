package circuitbreaker_test

import (
	"errors"
	"testing"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/circuitbreaker"
	"github.com/sony/gobreaker/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCircuitBreaker_SuccessPassThrough(t *testing.T) {
	cb := circuitbreaker.New[string](circuitbreaker.DefaultConfig("test"))

	result, err := cb.Execute(func() (string, error) {
		return "ok", nil
	})

	require.NoError(t, err)
	assert.Equal(t, "ok", result)
}

func TestCircuitBreaker_PropagatesError(t *testing.T) {
	cb := circuitbreaker.New[string](circuitbreaker.DefaultConfig("test"))

	_, err := cb.Execute(func() (string, error) {
		return "", errors.New("downstream error")
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "downstream error")
}

func TestCircuitBreaker_TripsAfterThreshold(t *testing.T) {
	cfg := circuitbreaker.Config{
		Name:        "trip-test",
		MaxRequests: 1,
		Threshold:   0.5,
		Interval:    0,
		Timeout:     60 * time.Second,
	}
	cb := circuitbreaker.New[string](cfg)

	// Trigger 6 failures to exceed minimum request count and threshold
	for i := 0; i < 6; i++ {
		_, _ = cb.Execute(func() (string, error) {
			return "", errors.New("fail")
		})
	}

	// Circuit should now be open
	_, err := cb.Execute(func() (string, error) {
		return "should not reach", nil
	})

	require.Error(t, err)
	assert.Equal(t, gobreaker.ErrOpenState, err)
}

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	cfg := circuitbreaker.DefaultConfig("my-service")
	assert.Equal(t, "my-service", cfg.Name)
	assert.Equal(t, uint32(3), cfg.MaxRequests)
	assert.Equal(t, 0.5, cfg.Threshold)
}
