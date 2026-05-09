package logger

import (
	"context"
	"io"
	"os"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type contextKey string

const (
	traceIDKey   contextKey = "trace_id"
	requestIDKey contextKey = "request_id"
	userIDKey    contextKey = "user_id"
)

var sensitiveFields = map[string]bool{
	"password":      true,
	"password_hash": true,
	"token":         true,
	"secret":        true,
	"authorization": true,
}

type Config struct {
	Service string
	Level   string
	Pretty  bool
	Output  io.Writer
}

type Logger interface {
	Info(ctx context.Context) *zerolog.Event
	Warn(ctx context.Context) *zerolog.Event
	Error(ctx context.Context) *zerolog.Event
	Debug(ctx context.Context) *zerolog.Event
	Fatal(ctx context.Context) *zerolog.Event
	With(key, value string) Logger
}

type logger struct {
	zl zerolog.Logger
}

func New(cfg Config) Logger {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}

	out := cfg.Output
	if out == nil {
		out = os.Stdout
	}

	var w io.Writer = out
	if cfg.Pretty {
		w = zerolog.ConsoleWriter{Out: out}
	}

	zl := zerolog.New(w).
		Level(level).
		With().
		Str("service", cfg.Service).
		Timestamp().
		Logger()

	return &logger{zl: zl}
}

func GenerateRequestID() string {
	return uuid.New().String()
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func UserIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(userIDKey).(string); ok {
		return id
	}
	return ""
}

// WithTraceID stores a trace ID in the context (legacy, alias for WithRequestID).
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

func IsSensitiveField(field string) bool {
	return sensitiveFields[field]
}

func MaskSensitive(fields map[string]any) map[string]any {
	masked := make(map[string]any, len(fields))
	for k, v := range fields {
		if sensitiveFields[k] {
			masked[k] = "[REDACTED]"
		} else {
			masked[k] = v
		}
	}
	return masked
}

func (l *logger) event(ctx context.Context, e *zerolog.Event) *zerolog.Event {
	if traceID, ok := ctx.Value(traceIDKey).(string); ok && traceID != "" {
		e = e.Str("trace_id", traceID)
	}
	if requestID, ok := ctx.Value(requestIDKey).(string); ok && requestID != "" {
		e = e.Str("request_id", requestID)
	}
	if userID, ok := ctx.Value(userIDKey).(string); ok && userID != "" {
		e = e.Str("user_id", userID)
	}
	return e
}

func (l *logger) Info(ctx context.Context) *zerolog.Event {
	return l.event(ctx, l.zl.Info())
}

func (l *logger) Warn(ctx context.Context) *zerolog.Event {
	return l.event(ctx, l.zl.Warn())
}

func (l *logger) Error(ctx context.Context) *zerolog.Event {
	return l.event(ctx, l.zl.Error())
}

func (l *logger) Debug(ctx context.Context) *zerolog.Event {
	return l.event(ctx, l.zl.Debug())
}

func (l *logger) Fatal(ctx context.Context) *zerolog.Event {
	return l.event(ctx, l.zl.Fatal())
}

func (l *logger) With(key, value string) Logger {
	return &logger{zl: l.zl.With().Str(key, value).Logger()}
}
