package logger

import (
	"context"
	"io"
	"os"

	"github.com/rs/zerolog"
)

type contextKey string

const traceIDKey contextKey = "trace_id"

type Config struct {
	Service string
	Level   string    // "debug", "info", "warn", "error"
	Pretty  bool      // true = human-readable (dev), false = JSON (prod)
	Output  io.Writer // nil = os.Stdout
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

// WithTraceID stores a trace ID in the context.
// When OTel is integrated, this will be replaced by OTel span context extraction.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey, traceID)
}

func (l *logger) event(ctx context.Context, e *zerolog.Event) *zerolog.Event {
	if traceID, ok := ctx.Value(traceIDKey).(string); ok && traceID != "" {
		e = e.Str("trace_id", traceID)
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
