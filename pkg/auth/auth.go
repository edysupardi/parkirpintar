package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

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

func (v *Validator) GenerateToken(userID, vehicleType string, expiryHours int) (string, error) {
	claims := &Claims{
		UserID:      userID,
		VehicleType: vehicleType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(v.secret)
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

// HTTPMiddleware validates JWT from HTTP Authorization header and injects user_id into context.
// Used with grpc-gateway's RegisterHandlerServer (in-process, no gRPC interceptor).
func (v *Validator) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || r.URL.Path == "/swagger.json" ||
			strings.HasPrefix(r.URL.Path, "/swagger/") ||
			r.URL.Path == "/v1/auth/register" || r.URL.Path == "/v1/auth/login" {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"code":16,"message":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := v.ParseToken(tokenStr)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"code":16,"message":"invalid token: %v"}`, err), http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
