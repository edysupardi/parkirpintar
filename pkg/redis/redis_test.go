package redis_test

import (
	"testing"

	"github.com/edysupardi/parkirpintar/pkg/redis"
	"github.com/stretchr/testify/assert"
)

func TestConfig_Validate_MissingHost(t *testing.T) {
	cfg := redis.Config{Port: 6379}
	err := cfg.ExportValidate()
	assert.ErrorContains(t, err, "host")
}

func TestConfig_Validate_MissingPort(t *testing.T) {
	cfg := redis.Config{Host: "localhost"}
	err := cfg.ExportValidate()
	assert.ErrorContains(t, err, "port")
}

func TestConfig_Validate_Valid(t *testing.T) {
	cfg := redis.Config{Host: "localhost", Port: 6379}
	err := cfg.ExportValidate()
	assert.NoError(t, err)
}

func TestConfig_Addr(t *testing.T) {
	cfg := redis.Config{Host: "localhost", Port: 6379}
	assert.Equal(t, "localhost:6379", cfg.ExportAddr())
}
