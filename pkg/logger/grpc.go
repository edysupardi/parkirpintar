package logger

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func UnaryServerLogger(log Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()

		requestID := requestIDFromMetadata(ctx)
		if requestID == "" {
			requestID = GenerateRequestID()
		}
		ctx = WithRequestID(ctx, requestID)

		resp, err := handler(ctx, req)

		latency := time.Since(start)
		st, _ := status.FromError(err)

		event := log.Info(ctx)
		if err != nil {
			event = log.Error(ctx).Err(err)
		}

		event.
			Str("method", info.FullMethod).
			Str("status", st.Code().String()).
			Dur("latency", latency).
			Msg("grpc request")

		return resp, err
	}
}

func requestIDFromMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get("x-request-id")
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
