package circuitbreaker

import (
	"time"

	"github.com/sony/gobreaker/v2"
)

type Config struct {
	Name        string
	MaxRequests uint32
	Interval    time.Duration
	Timeout     time.Duration
	Threshold   float64 // failure ratio to trip (0.0–1.0)
}

func DefaultConfig(name string) Config {
	return Config{
		Name:        name,
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		Threshold:   0.5,
	}
}

func New[T any](cfg Config) *gobreaker.CircuitBreaker[T] {
	return gobreaker.NewCircuitBreaker[T](gobreaker.Settings{
		Name:        cfg.Name,
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			if counts.Requests < 5 {
				return false
			}
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return failureRatio >= cfg.Threshold
		},
	})
}
