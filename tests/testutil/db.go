package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type TestDB struct {
	Pool      *pgxpool.Pool
	container testcontainers.Container
}

func NewTestDB(ctx context.Context) (*TestDB, error) {
	pgContainer, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("parkirpintar_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("start postgres container: %w", err)
	}

	connStr, err := pgContainer.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("get connection string: %w", err)
	}

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := runMigrations(ctx, pool); err != nil {
		pool.Close()
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	if err := seedSpots(ctx, pool); err != nil {
		pool.Close()
		pgContainer.Terminate(ctx)
		return nil, fmt.Errorf("seed spots: %w", err)
	}

	return &TestDB{Pool: pool, container: pgContainer}, nil
}

func (db *TestDB) Close(ctx context.Context) {
	db.Pool.Close()
	db.container.Terminate(ctx)
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	migrationsDir := findMigrationsDir()
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir %s: %w", migrationsDir, err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".sql" && len(e.Name()) > 7 && e.Name()[len(e.Name())-7:] == ".up.sql" {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for _, f := range upFiles {
		sql, err := os.ReadFile(filepath.Join(migrationsDir, f))
		if err != nil {
			return fmt.Errorf("read %s: %w", f, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("exec %s: %w", f, err)
		}
	}
	return nil
}

func seedSpots(ctx context.Context, pool *pgxpool.Pool) error {
	for floor := int32(1); floor <= 5; floor++ {
		for num := int32(1); num <= 30; num++ {
			_, err := pool.Exec(ctx,
				`INSERT INTO spots (id, floor, number, vehicle_type, status) VALUES (gen_random_uuid(), $1, $2, 'car', 'available') ON CONFLICT DO NOTHING`,
				floor, num)
			if err != nil {
				return err
			}
		}
		for num := int32(1); num <= 50; num++ {
			_, err := pool.Exec(ctx,
				`INSERT INTO spots (id, floor, number, vehicle_type, status) VALUES (gen_random_uuid(), $1, $2, 'motorcycle', 'available') ON CONFLICT DO NOTHING`,
				floor, num)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func findMigrationsDir() string {
	candidates := []string{
		"migrations",
		"../../migrations",
		"../../../migrations",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return "migrations"
}
