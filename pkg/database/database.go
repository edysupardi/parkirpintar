package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	Host         string
	Port         int
	Name         string
	User         string
	Password     string
	SSLMode      string
	MaxOpenConns int32
	MaxIdleConns int32
}

func (c Config) ExportDSN() string { return c.dsn() }
func (c Config) ExportValidate() error { return c.validate() }

func (c Config) dsn() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s pool_max_conns=%d pool_min_conns=%d",
		c.Host, c.Port, c.Name, c.User, c.Password, c.SSLMode, c.MaxOpenConns, c.MaxIdleConns,
	)
}

func (c Config) validate() error {
	if c.Host == "" {
		return fmt.Errorf("database host is required")
	}
	if c.Name == "" {
		return fmt.Errorf("database name is required")
	}
	if c.User == "" {
		return fmt.Errorf("database user is required")
	}
	if c.Password == "" {
		return fmt.Errorf("database password is required")
	}
	return nil
}

type DB struct {
	pool *pgxpool.Pool
}

func New(ctx context.Context, cfg Config) (*DB, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	pool, err := pgxpool.New(ctx, cfg.dsn())
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	return &DB{pool: pool}, nil
}

func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

func (db *DB) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

func (db *DB) Close() {
	db.pool.Close()
}
