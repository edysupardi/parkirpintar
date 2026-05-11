package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()
	connStr := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		getenv("DB_USER", "postgres"),
		getenv("DB_PASSWORD", "secret"),
		getenv("DB_HOST", "localhost"),
		getenv("DB_PORT", "5432"),
		getenv("DB_NAME", "parkirpintar"),
		getenv("DB_SSL_MODE", "disable"),
	)

	conn, err := pgx.Connect(ctx, connStr)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close(ctx)

	count := 0
	// 5 floors: 30 cars + 50 motorcycles each
	for floor := 1; floor <= 5; floor++ {
		for num := 1; num <= 30; num++ {
			_, err = conn.Exec(ctx,
				`INSERT INTO spots (floor, number, vehicle_type)
				VALUES ($1, $2, 'car') ON CONFLICT DO NOTHING`,
				floor, num,
			)
			if err != nil { log.Fatalf("seed car error: %v", err) }
			count++
		}
		for num := 1; num <= 50; num++ {
			_, err = conn.Exec(ctx,
				`INSERT INTO spots (floor, number, vehicle_type)
				VALUES ($1, $2, 'motorcycle') ON CONFLICT DO NOTHING`,
				floor, num,
			)
			if err != nil { log.Fatalf("seed motorcycle error: %v", err) }
			count++
		}
	}

	log.Printf("Seeded %d spots successfully", count)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" { return v }
	return fallback
}
