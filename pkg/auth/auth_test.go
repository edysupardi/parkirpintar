package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/auth"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const testSecret = "test-secret-key"

func makeToken(t *testing.T, userID string, secret string, exp time.Time) string {
	t.Helper()
	claims := auth.Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestValidator_ParseToken_Valid(t *testing.T) {
	v := auth.New(testSecret)
	tokenStr := makeToken(t, "user-123", testSecret, time.Now().Add(time.Hour))
	claims, err := v.ParseToken(tokenStr)
	assert.NoError(t, err)
	assert.Equal(t, "user-123", claims.UserID)
}

func TestValidator_ParseToken_WrongSecret(t *testing.T) {
	v := auth.New(testSecret)
	tokenStr := makeToken(t, "user-123", "wrong-secret", time.Now().Add(time.Hour))
	_, err := v.ParseToken(tokenStr)
	assert.Error(t, err)
}

func TestValidator_ParseToken_Expired(t *testing.T) {
	v := auth.New(testSecret)
	tokenStr := makeToken(t, "user-123", testSecret, time.Now().Add(-time.Hour))
	_, err := v.ParseToken(tokenStr)
	assert.Error(t, err)
}

func TestValidator_UnaryInterceptor_Valid(t *testing.T) {
	v := auth.New(testSecret)
	tokenStr := makeToken(t, "user-123", testSecret, time.Now().Add(time.Hour))

	md := metadata.Pairs("authorization", "Bearer "+tokenStr)
	ctx := metadata.NewIncomingContext(context.Background(), md)

	called := false
	_, err := v.UnaryInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/reservation.v1.Reservation/Create"}, func(ctx context.Context, req any) (any, error) {
		called = true
		id, ok := auth.UserIDFromContext(ctx)
		assert.True(t, ok)
		assert.Equal(t, "user-123", id)
		return nil, nil
	})
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestValidator_UnaryInterceptor_MissingToken(t *testing.T) {
	v := auth.New(testSecret)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.MD{})

	_, err := v.UnaryInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/reservation.v1.Reservation/Create"}, nil)
	assert.Error(t, err)
}

func TestValidator_UnaryInterceptor_SkipsPublicMethod(t *testing.T) {
	v := auth.New(testSecret)
	ctx := context.Background()

	called := false
	_, err := v.UnaryInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"}, func(ctx context.Context, req any) (any, error) {
		called = true
		return nil, nil
	})
	assert.NoError(t, err)
	assert.True(t, called)
}
