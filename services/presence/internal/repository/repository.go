package repository

import (
	"context"
	"errors"

	"github.com/edysupardi/parkirpintar/services/presence/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) SaveLocation(ctx context.Context, u domain.LocationUpdate) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO location_updates (driver_id, session_id, reservation_id, latitude, longitude, accuracy, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.DriverID, nullStr(u.SessionID), nullStr(u.ReservationID),
		u.Latitude, u.Longitude, u.Accuracy, u.RecordedAt,
	)
	return err
}

func (r *PostgresRepository) GetLastLocation(ctx context.Context, driverID string) (*domain.LocationUpdate, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT driver_id, COALESCE(session_id::text,''), COALESCE(reservation_id::text,''),
		       latitude, longitude, accuracy, recorded_at
		FROM location_updates
		WHERE driver_id = $1
		ORDER BY recorded_at DESC
		LIMIT 1`, driverID)

	var u domain.LocationUpdate
	err := row.Scan(&u.DriverID, &u.SessionID, &u.ReservationID,
		&u.Latitude, &u.Longitude, &u.Accuracy, &u.RecordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
