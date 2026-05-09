package logger

import (
	"net/http"
	"time"
)

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func HTTPRequestLogger(log Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = GenerateRequestID()
			}

			ctx := WithRequestID(r.Context(), requestID)
			r = r.WithContext(ctx)

			w.Header().Set("X-Request-ID", requestID)

			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			latency := time.Since(start)
			userID := UserIDFromContext(r.Context())

			event := log.Info(r.Context())
			if wrapped.statusCode >= 500 {
				event = log.Error(r.Context())
			} else if wrapped.statusCode >= 400 {
				event = log.Warn(r.Context())
			}

			event.
				Str("method", r.Method).
				Str("path", r.URL.Path).
				Int("status", wrapped.statusCode).
				Dur("latency", latency).
				Str("ip", clientIP(r)).
				Str("user_agent", r.UserAgent())

			if userID != "" {
				event.Str("user_id", userID)
			}

			event.Msg("http request")
		})
	}
}

func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		return forwarded
	}
	if real := r.Header.Get("X-Real-IP"); real != "" {
		return real
	}
	return r.RemoteAddr
}
