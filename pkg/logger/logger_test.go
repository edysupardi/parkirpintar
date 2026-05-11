package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger(buf *bytes.Buffer) logger.Logger {
	return logger.New(logger.Config{
		Service: "test-service",
		Level:   "debug",
		Output:  buf,
	})
}

func parseLog(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	return m
}

func TestLogger_InfoLevel(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)
	log.Info(context.Background()).Msg("hello")
	m := parseLog(t, &buf)
	assert.Equal(t, "info", m["level"])
	assert.Equal(t, "hello", m["message"])
	assert.Equal(t, "test-service", m["service"])
}

func TestLogger_With(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf).With("component", "usecase")
	log.Info(context.Background()).Msg("ok")
	m := parseLog(t, &buf)
	assert.Equal(t, "usecase", m["component"])
}

func TestLogger_WithReturnsNewInstance(t *testing.T) {
	var buf bytes.Buffer
	base := newTestLogger(&buf)
	child := base.With("component", "usecase")
	assert.NotEqual(t, base, child)
}

func TestLogger_TraceIDFromContext(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)
	ctx := logger.WithTraceID(context.Background(), "abc-123")
	log.Info(ctx).Msg("traced")
	m := parseLog(t, &buf)
	assert.Equal(t, "abc-123", m["trace_id"])
}

func TestLogger_NoTraceIDWhenContextEmpty(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)
	log.Info(context.Background()).Msg("no trace")
	m := parseLog(t, &buf)
	_, exists := m["trace_id"]
	assert.False(t, exists)
}

func TestLogger_DebugLevel(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)
	log.Debug(context.Background()).Msg("debug msg")
	m := parseLog(t, &buf)
	assert.Equal(t, "debug", m["level"])
}

func TestLogger_WarnLevel(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)
	log.Warn(context.Background()).Msg("warn msg")
	m := parseLog(t, &buf)
	assert.Equal(t, "warn", m["level"])
}

func TestLogger_ErrorLevel(t *testing.T) {
	var buf bytes.Buffer
	log := newTestLogger(&buf)
	log.Error(context.Background()).Msg("error msg")
	m := parseLog(t, &buf)
	assert.Equal(t, "error", m["level"])
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(logger.Config{
		Service: "test",
		Level:   "warn",
		Output:  &buf,
	})
	log.Debug(context.Background()).Msg("should not appear")
	assert.Empty(t, buf.Bytes())
}
