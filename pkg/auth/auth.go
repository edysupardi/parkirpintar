package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type contextKey string

const userIDKey contextKey = "user_id"

type Claims struct {
	UserID      string `json:"user_id"`
	VehicleType string `json:"vehicle_type"`
	jwt.RegisteredClaims
}

type Validator struct {
	secret []byte
}

func New(secret string) *Validator {
	return &Validator{secret: []byte(secret)}
}

func (v *Validator) ParseToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return v.secret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}
	return claims, nil
}

// UnaryInterceptor validates JWT from gRPC metadata and injects user_id into context.
func (v *Validator) UnaryInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if isPublicMethod(info.FullMethod) {
		return handler(ctx, req)
	}

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	tokenStr := strings.TrimPrefix(values[0], "Bearer ")
	claims, err := v.ParseToken(tokenStr)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
	}

	ctx = context.WithValue(ctx, userIDKey, claims.UserID)
	return handler(ctx, req)
}

// UserIDFromContext extracts the user_id injected by the interceptor.
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(userIDKey).(string)
	return id, ok && id != ""
}

var publicMethods = map[string]bool{
	"/grpc.health.v1.Health/Check": true,
}

func isPublicMethod(method string) bool {
	return publicMethods[method]
}
