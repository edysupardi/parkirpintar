package config_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/edysupardi/parkirpintar/pkg/config"
)

// TestLoad_WithDefaults memastikan config bisa load dengan nilai default
// tanpa .env file, selama required vars sudah diset.
func TestLoad_WithDefaults(t *testing.T) {
	// Set required env vars yang tidak punya default
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("JWT_SECRET", "test-secret-minimum-32-characters-long")

	cfg, err := config.Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	// Verifikasi default values
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "parkirpintar", cfg.Database.Name)
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, "amqp://guest:guest@localhost:5672/", cfg.RabbitMQ.URL)
	assert.Equal(t, 8080, cfg.Services.GatewayHTTPPort)
	assert.Equal(t, 9001, cfg.Services.ReservationGRPCPort)
	assert.Equal(t, 50.0, cfg.Parking.GeofenceRadiusMeters)
	assert.Equal(t, 60, cfg.Parking.ReservationHoldMins)
	assert.Equal(t, "stub", cfg.Feature.NotificationProvider)
	assert.Equal(t, "mock", cfg.Feature.PaymentProvider)
}

// TestLoad_MissingDBPassword memastikan error kalau DB_PASSWORD tidak diset.
func TestLoad_MissingDBPassword(t *testing.T) {
	// Pastikan env var tidak ada
	os.Unsetenv("DB_PASSWORD")
	t.Setenv("JWT_SECRET", "test-secret-minimum-32-characters-long")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_PASSWORD")
}

// TestLoad_MissingJWTSecret memastikan error kalau JWT_SECRET tidak diset.
func TestLoad_MissingJWTSecret(t *testing.T) {
	t.Setenv("DB_PASSWORD", "secret")
	os.Unsetenv("JWT_SECRET")

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JWT_SECRET")
}

// TestLoad_MidtransValidation memastikan Midtrans keys wajib ada
// hanya kalau PAYMENT_PROVIDER=midtrans.
func TestLoad_MidtransValidation(t *testing.T) {
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("JWT_SECRET", "test-secret-minimum-32-characters-long")
	t.Setenv("PAYMENT_PROVIDER", "midtrans")
	// Sengaja tidak set MIDTRANS_SERVER_KEY dan CLIENT_KEY

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MIDTRANS_SERVER_KEY")
}

// TestLoad_MidtransSkippedWhenMock memastikan Midtrans keys tidak wajib
// kalau PAYMENT_PROVIDER=mock (untuk testing).
func TestLoad_MidtransSkippedWhenMock(t *testing.T) {
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("JWT_SECRET", "test-secret-minimum-32-characters-long")
	t.Setenv("PAYMENT_PROVIDER", "mock")
	// Tidak set MIDTRANS keys — harusnya tidak error

	cfg, err := config.Load()
	require.NoError(t, err)
	assert.Equal(t, "mock", cfg.Feature.PaymentProvider)
}

// TestLoad_FCMValidation memastikan FCM_PROJECT_ID wajib ada
// hanya kalau NOTIFICATION_PROVIDER=fcm atau all.
func TestLoad_FCMValidation(t *testing.T) {
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("JWT_SECRET", "test-secret-minimum-32-characters-long")
	t.Setenv("NOTIFICATION_PROVIDER", "fcm")
	// Sengaja tidak set FCM_PROJECT_ID

	_, err := config.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "FCM_PROJECT_ID")
}

// TestLoad_EnvOverride memastikan env vars override default values.
func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("DB_PASSWORD", "secret")
	t.Setenv("JWT_SECRET", "test-secret-minimum-32-characters-long")
	t.Setenv("DB_HOST", "custom-db-host")
	t.Setenv("DB_PORT", "5433")
	t.Setenv("REDIS_ADDR", "redis-custom:6380")
	t.Setenv("GEOFENCE_RADIUS_METERS", "100.0")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "custom-db-host", cfg.Database.Host)
	assert.Equal(t, 5433, cfg.Database.Port)
	assert.Equal(t, "redis-custom:6380", cfg.Redis.Addr)
	assert.Equal(t, 100.0, cfg.Parking.GeofenceRadiusMeters)
}

// TestDatabaseDSN memastikan DSN format sudah benar.
func TestDatabaseDSN(t *testing.T) {
	t.Setenv("DB_PASSWORD", "mypassword")
	t.Setenv("JWT_SECRET", "test-secret-minimum-32-characters-long")

	cfg, err := config.Load()
	require.NoError(t, err)

	dsn := cfg.Database.DSN()
	assert.Contains(t, dsn, "host=localhost")
	assert.Contains(t, dsn, "port=5432")
	assert.Contains(t, dsn, "dbname=parkirpintar")
	assert.Contains(t, dsn, "password=mypassword")
	assert.Contains(t, dsn, "sslmode=disable")
}