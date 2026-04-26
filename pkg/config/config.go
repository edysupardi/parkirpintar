package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config adalah struct utama yang berisi semua konfigurasi aplikasi.
// Semua service di ParkirPintar menggunakan package ini.
type Config struct {
	Database     DatabaseConfig
	Redis        RedisConfig
	RabbitMQ     RabbitMQConfig
	JWT          JWTConfig
	Services     ServicesConfig
	Midtrans     MidtransConfig
	FCM          FCMConfig
	SES          SESConfig
	Parking      ParkingConfig
	Feature      FeatureConfig
}

// DatabaseConfig menyimpan konfigurasi koneksi PostgreSQL (Aurora).
type DatabaseConfig struct {
	Host            string
	Port            int
	Name            string
	User            string
	Password        string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnTimeout     time.Duration
}

// DSN mengembalikan PostgreSQL connection string.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s connect_timeout=%d",
		d.Host,
		d.Port,
		d.Name,
		d.User,
		d.Password,
		d.SSLMode,
		int(d.ConnTimeout.Seconds()),
	)
}

// RedisConfig menyimpan konfigurasi koneksi Redis (ElastiCache).
type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

// RabbitMQConfig menyimpan konfigurasi koneksi Amazon MQ (RabbitMQ).
type RabbitMQConfig struct {
	URL string
}

// JWTConfig menyimpan konfigurasi JWT untuk auth interceptor.
type JWTConfig struct {
	Secret      string
	ExpiryHours int
}

// ServicesConfig menyimpan port untuk setiap gRPC service.
type ServicesConfig struct {
	GatewayHTTPPort      int
	GatewayGRPCPort      int
	ReservationGRPCPort  int
	BillingGRPCPort      int
	PaymentGRPCPort      int
	PresenceGRPCPort     int
	NotificationGRPCPort int
}

// MidtransConfig menyimpan konfigurasi Midtrans payment gateway.
type MidtransConfig struct {
	ServerKey string
	ClientKey string
	Env       string // "sandbox" atau "production"
}

// FCMConfig menyimpan konfigurasi Firebase Cloud Messaging.
type FCMConfig struct {
	ProjectID       string
	CredentialsFile string
}

// SESConfig menyimpan konfigurasi Amazon SES untuk email.
type SESConfig struct {
	Region    string
	FromEmail string
}

// ParkingConfig menyimpan konfigurasi area parkir.
type ParkingConfig struct {
	Lat                  float64
	Lng                  float64
	GeofenceRadiusMeters float64
	ReservationHoldMins  int
}

// FeatureConfig menyimpan feature flags.
// Memungkinkan switch antara real dan stub implementation.
type FeatureConfig struct {
	// "stub" | "fcm" | "ses" | "all"
	NotificationProvider string
	// "mock" | "midtrans"
	PaymentProvider string
}

// Load membaca konfigurasi dari environment variables dan .env file.
// Urutan prioritas: env vars > .env file > default values.
func Load() (*Config, error) {
	v := viper.New()

	// Baca dari file .env di working directory
	v.SetConfigName(".env")
	v.SetConfigType("env")
	v.AddConfigPath(".")
	v.AddConfigPath("../../") // untuk service yang ada di services/{name}/

	// Baca environment variables secara otomatis
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Baca .env file kalau ada — tidak error kalau tidak ada
	// karena bisa saja konfigurasi 100% dari env vars (production)
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
	}

	setDefaults(v)

	cfg := &Config{}
	if err := bindAll(v, cfg); err != nil {
		return nil, fmt.Errorf("error binding config: %w", err)
	}

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

// setDefaults mendefinisikan nilai default untuk setiap konfigurasi.
// Nilai ini dipakai kalau env var atau .env tidak mendefinisikannya.
func setDefaults(v *viper.Viper) {
	// Database
	v.SetDefault("DB_HOST", "localhost")
	v.SetDefault("DB_PORT", 5432)
	v.SetDefault("DB_NAME", "parkirpintar")
	v.SetDefault("DB_USER", "postgres")
	v.SetDefault("DB_SSL_MODE", "disable")
	v.SetDefault("DB_MAX_OPEN_CONNS", 25)
	v.SetDefault("DB_MAX_IDLE_CONNS", 5)
	v.SetDefault("DB_CONN_TIMEOUT_SECONDS", 5)

	// Redis
	v.SetDefault("REDIS_ADDR", "localhost:6379")
	v.SetDefault("REDIS_DB", 0)

	// RabbitMQ
	v.SetDefault("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")

	// JWT
	v.SetDefault("JWT_EXPIRY_HOURS", 24)

	// Service ports
	v.SetDefault("GATEWAY_HTTP_PORT", 8080)
	v.SetDefault("GATEWAY_GRPC_PORT", 9000)
	v.SetDefault("RESERVATION_GRPC_PORT", 9001)
	v.SetDefault("BILLING_GRPC_PORT", 9002)
	v.SetDefault("PAYMENT_GRPC_PORT", 9003)
	v.SetDefault("PRESENCE_GRPC_PORT", 9004)
	v.SetDefault("NOTIFICATION_GRPC_PORT", 9005)

	// Midtrans
	v.SetDefault("MIDTRANS_ENV", "sandbox")

	// AWS
	v.SetDefault("AWS_REGION", "ap-southeast-1")

	// Parking
	v.SetDefault("PARKING_LAT", -6.2088)
	v.SetDefault("PARKING_LNG", 106.8456)
	v.SetDefault("GEOFENCE_RADIUS_METERS", 50.0)
	v.SetDefault("RESERVATION_HOLD_MINUTES", 60)

	// Feature flags
	v.SetDefault("NOTIFICATION_PROVIDER", "stub")
	v.SetDefault("PAYMENT_PROVIDER", "mock")
}

// bindAll memetakan semua viper values ke Config struct.
func bindAll(v *viper.Viper, cfg *Config) error {
	cfg.Database = DatabaseConfig{
		Host:         v.GetString("DB_HOST"),
		Port:         v.GetInt("DB_PORT"),
		Name:         v.GetString("DB_NAME"),
		User:         v.GetString("DB_USER"),
		Password:     v.GetString("DB_PASSWORD"),
		SSLMode:      v.GetString("DB_SSL_MODE"),
		MaxOpenConns: v.GetInt("DB_MAX_OPEN_CONNS"),
		MaxIdleConns: v.GetInt("DB_MAX_IDLE_CONNS"),
		ConnTimeout:  time.Duration(v.GetInt("DB_CONN_TIMEOUT_SECONDS")) * time.Second,
	}

	cfg.Redis = RedisConfig{
		Addr:     v.GetString("REDIS_ADDR"),
		Password: v.GetString("REDIS_PASSWORD"),
		DB:       v.GetInt("REDIS_DB"),
	}

	cfg.RabbitMQ = RabbitMQConfig{
		URL: v.GetString("RABBITMQ_URL"),
	}

	cfg.JWT = JWTConfig{
		Secret:      v.GetString("JWT_SECRET"),
		ExpiryHours: v.GetInt("JWT_EXPIRY_HOURS"),
	}

	cfg.Services = ServicesConfig{
		GatewayHTTPPort:      v.GetInt("GATEWAY_HTTP_PORT"),
		GatewayGRPCPort:      v.GetInt("GATEWAY_GRPC_PORT"),
		ReservationGRPCPort:  v.GetInt("RESERVATION_GRPC_PORT"),
		BillingGRPCPort:      v.GetInt("BILLING_GRPC_PORT"),
		PaymentGRPCPort:      v.GetInt("PAYMENT_GRPC_PORT"),
		PresenceGRPCPort:     v.GetInt("PRESENCE_GRPC_PORT"),
		NotificationGRPCPort: v.GetInt("NOTIFICATION_GRPC_PORT"),
	}

	cfg.Midtrans = MidtransConfig{
		ServerKey: v.GetString("MIDTRANS_SERVER_KEY"),
		ClientKey: v.GetString("MIDTRANS_CLIENT_KEY"),
		Env:       v.GetString("MIDTRANS_ENV"),
	}

	cfg.FCM = FCMConfig{
		ProjectID:       v.GetString("FCM_PROJECT_ID"),
		CredentialsFile: v.GetString("FCM_CREDENTIALS_FILE"),
	}

	cfg.SES = SESConfig{
		Region:    v.GetString("AWS_REGION"),
		FromEmail: v.GetString("SES_FROM_EMAIL"),
	}

	cfg.Parking = ParkingConfig{
		Lat:                  v.GetFloat64("PARKING_LAT"),
		Lng:                  v.GetFloat64("PARKING_LNG"),
		GeofenceRadiusMeters: v.GetFloat64("GEOFENCE_RADIUS_METERS"),
		ReservationHoldMins:  v.GetInt("RESERVATION_HOLD_MINUTES"),
	}

	cfg.Feature = FeatureConfig{
		NotificationProvider: v.GetString("NOTIFICATION_PROVIDER"),
		PaymentProvider:      v.GetString("PAYMENT_PROVIDER"),
	}

	return nil
}

// validate memastikan semua required config sudah diisi.
// Config yang tidak ada default dan wajib ada dicek di sini.
func validate(cfg *Config) error {
	var missing []string

	if cfg.Database.Password == "" {
		missing = append(missing, "DB_PASSWORD")
	}

	if cfg.JWT.Secret == "" {
		missing = append(missing, "JWT_SECRET")
	}

	// Validasi Midtrans hanya kalau payment provider bukan mock
	if cfg.Feature.PaymentProvider == "midtrans" {
		if cfg.Midtrans.ServerKey == "" {
			missing = append(missing, "MIDTRANS_SERVER_KEY")
		}
		if cfg.Midtrans.ClientKey == "" {
			missing = append(missing, "MIDTRANS_CLIENT_KEY")
		}
	}

	// Validasi FCM hanya kalau notification provider butuh FCM
	if cfg.Feature.NotificationProvider == "fcm" || cfg.Feature.NotificationProvider == "all" {
		if cfg.FCM.ProjectID == "" {
			missing = append(missing, "FCM_PROJECT_ID")
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required config: %s", strings.Join(missing, ", "))
	}

	return nil
}