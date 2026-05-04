package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/edysupardi/parkirpintar/services/reservation/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) InsertReservation(ctx context.Context, res domain.Reservation) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO reservations
			(id, driver_id, spot_id, status, assignment_mode, idempotency_key, confirmed_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		res.ReservationID, res.DriverID, res.Spot.SpotID,
		string(res.Status), string(res.AssignmentMode),
		res.IdempotencyKey, res.ConfirmedAt, res.ExpiresAt,
	)
	return err
}

func (r *PostgresRepository) GetReservation(ctx context.Context, reservationID string) (*domain.Reservation, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT r.id, r.driver_id, r.status, r.assignment_mode, r.idempotency_key,
		       r.confirmed_at, r.expires_at, r.check_in_at, r.check_out_at, r.cancelled_at, r.session_id,
		       s.id, s.floor, s.number, s.vehicle_type
		FROM reservations r
		JOIN spots s ON s.id = r.spot_id
		WHERE r.id = $1`, reservationID)
	return scanReservation(row)
}

func (r *PostgresRepository) GetActiveReservation(ctx context.Context, driverID string) (*domain.Reservation, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT r.id, r.driver_id, r.status, r.assignment_mode, r.idempotency_key,
		       r.confirmed_at, r.expires_at, r.check_in_at, r.check_out_at, r.cancelled_at, r.session_id,
		       s.id, s.floor, s.number, s.vehicle_type
		FROM reservations r
		JOIN spots s ON s.id = r.spot_id
		WHERE r.driver_id = $1 AND r.status IN ('confirmed', 'active')
		LIMIT 1`, driverID)
	res, err := scanReservation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return res, err
}

func (r *PostgresRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Reservation, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT r.id, r.driver_id, r.status, r.assignment_mode, r.idempotency_key,
		       r.confirmed_at, r.expires_at, r.check_in_at, r.check_out_at, r.cancelled_at, r.session_id,
		       s.id, s.floor, s.number, s.vehicle_type
		FROM reservations r
		JOIN spots s ON s.id = r.spot_id
		WHERE r.idempotency_key = $1`, key)
	return scanReservation(row)
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, reservationID string, status domain.ReservationStatus) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE reservations SET status = $1 WHERE id = $2`,
		string(status), reservationID)
	return err
}

func (r *PostgresRepository) UpdateCheckIn(ctx context.Context, reservationID, sessionID string, checkInAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE reservations SET status = 'active', session_id = $1, check_in_at = $2 WHERE id = $3`,
		sessionID, checkInAt, reservationID)
	return err
}

func (r *PostgresRepository) UpdateCheckOut(ctx context.Context, reservationID string, checkOutAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE reservations SET status = 'completed', check_out_at = $1 WHERE id = $2`,
		checkOutAt, reservationID)
	return err
}

func (r *PostgresRepository) UpdateCancelled(ctx context.Context, reservationID string, cancelledAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE reservations SET status = 'cancelled', cancelled_at = $1 WHERE id = $2`,
		cancelledAt, reservationID)
	return err
}

func (r *PostgresRepository) FindAvailableSpot(ctx context.Context, vehicleType domain.VehicleType) (*domain.Spot, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, floor, number, vehicle_type
		FROM spots
		WHERE vehicle_type = $1 AND status = 'available'
		ORDER BY floor ASC, number ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED`,
		string(vehicleType))

	var s domain.Spot
	var vt string
	err := row.Scan(&s.SpotID, &s.Floor, &s.SpotNumber, &vt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.VehicleType = domain.VehicleType(vt)
	return &s, nil
}

func (r *PostgresRepository) GetSpot(ctx context.Context, spotID string) (*domain.Spot, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, floor, number, vehicle_type FROM spots WHERE id = $1 AND status = 'available'`,
		spotID)

	var s domain.Spot
	var vt string
	err := row.Scan(&s.SpotID, &s.Floor, &s.SpotNumber, &vt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.VehicleType = domain.VehicleType(vt)
	return &s, nil
}

func (r *PostgresRepository) CountAvailable(ctx context.Context, vehicleType domain.VehicleType) (int32, error) {
	var count int32
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM spots WHERE vehicle_type = $1 AND status = 'available'`,
		string(vehicleType)).Scan(&count)
	return count, err
}

func (r *PostgresRepository) ListAvailableSpots(ctx context.Context, vehicleType domain.VehicleType, floor int32) ([]domain.Spot, error) {
	query := `SELECT id, floor, number, vehicle_type FROM spots WHERE vehicle_type = $1 AND status = 'available'`
	args := []any{string(vehicleType)}
	if floor > 0 {
		query += ` AND floor = $2`
		args = append(args, floor)
	}
	query += ` ORDER BY floor ASC, number ASC`

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var spots []domain.Spot
	for rows.Next() {
		var s domain.Spot
		var vt string
		if err := rows.Scan(&s.SpotID, &s.Floor, &s.SpotNumber, &vt); err != nil {
			return nil, err
		}
		s.VehicleType = domain.VehicleType(vt)
		spots = append(spots, s)
	}
	return spots, rows.Err()
}

func (r *PostgresRepository) ListExpiredReservations(ctx context.Context, before time.Time) ([]domain.Reservation, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.id, r.driver_id, r.status, r.assignment_mode, r.idempotency_key,
		       r.confirmed_at, r.expires_at, r.check_in_at, r.check_out_at, r.cancelled_at, r.session_id,
		       s.id, s.floor, s.number, s.vehicle_type
		FROM reservations r
		JOIN spots s ON s.id = r.spot_id
		WHERE r.status = 'confirmed' AND r.expires_at < $1`, before)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.Reservation
	for rows.Next() {
		res, err := scanReservation(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *res)
	}
	return result, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanReservation(row scanner) (*domain.Reservation, error) {
	var res domain.Reservation
	var status, mode, vt string
	err := row.Scan(
		&res.ReservationID, &res.DriverID, &status, &mode, &res.IdempotencyKey,
		&res.ConfirmedAt, &res.ExpiresAt, &res.CheckInAt, &res.CheckOutAt, &res.CancelledAt, &res.SessionID,
		&res.Spot.SpotID, &res.Spot.Floor, &res.Spot.SpotNumber, &vt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan reservation: %w", err)
	}
	res.Status = domain.ReservationStatus(status)
	res.AssignmentMode = domain.AssignmentMode(mode)
	res.Spot.VehicleType = domain.VehicleType(vt)
	return &res, nil
}
