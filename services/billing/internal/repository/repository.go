package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/edysupardi/parkirpintar/services/billing/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) InsertInvoice(ctx context.Context, inv domain.Invoice) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO invoices
			(id, session_id, reservation_id, driver_id, booking_fee, parking_fee,
			 overnight_fee, total_amount, billed_hours, is_overnight, duration_mins,
			 status, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		inv.InvoiceID, inv.SessionID, inv.ReservationID, inv.DriverID,
		inv.BookingFee, inv.ParkingFee, inv.OvernightFee, inv.TotalAmount,
		inv.BilledHours, inv.IsOvernight, inv.DurationMins,
		string(inv.Status), inv.IdempotencyKey,
	)
	return err
}

func (r *PostgresRepository) GetInvoice(ctx context.Context, invoiceID string) (*domain.Invoice, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, session_id, reservation_id, driver_id, booking_fee, parking_fee,
		       overnight_fee, total_amount, billed_hours, is_overnight, duration_mins,
		       status, gateway_tx_id, payment_method, idempotency_key, created_at, paid_at
		FROM invoices WHERE id = $1`, invoiceID)
	return scanInvoice(row)
}

func (r *PostgresRepository) GetInvoiceBySession(ctx context.Context, sessionID string) (*domain.Invoice, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, session_id, reservation_id, driver_id, booking_fee, parking_fee,
		       overnight_fee, total_amount, billed_hours, is_overnight, duration_mins,
		       status, gateway_tx_id, payment_method, idempotency_key, created_at, paid_at
		FROM invoices WHERE session_id = $1`, sessionID)
	return scanInvoice(row)
}

func (r *PostgresRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Invoice, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, session_id, reservation_id, driver_id, booking_fee, parking_fee,
		       overnight_fee, total_amount, billed_hours, is_overnight, duration_mins,
		       status, gateway_tx_id, payment_method, idempotency_key, created_at, paid_at
		FROM invoices WHERE idempotency_key = $1`, key)
	return scanInvoice(row)
}

func (r *PostgresRepository) MarkPaid(ctx context.Context, invoiceID, gatewayTxID, paymentMethod string, paidAt time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE invoices
		SET status = 'paid', gateway_tx_id = $1, payment_method = $2, paid_at = $3
		WHERE id = $4`,
		gatewayTxID, paymentMethod, paidAt, invoiceID)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanInvoice(row scanner) (*domain.Invoice, error) {
	var inv domain.Invoice
	var status string
	var gatewayTxID, paymentMethod *string
	err := row.Scan(
		&inv.InvoiceID, &inv.SessionID, &inv.ReservationID, &inv.DriverID,
		&inv.BookingFee, &inv.ParkingFee, &inv.OvernightFee, &inv.TotalAmount,
		&inv.BilledHours, &inv.IsOvernight, &inv.DurationMins,
		&status, &gatewayTxID, &paymentMethod, &inv.IdempotencyKey,
		&inv.CreatedAt, &inv.PaidAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("invoice not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scan invoice: %w", err)
	}
	inv.Status = domain.InvoiceStatus(status)
	if gatewayTxID != nil {
		inv.GatewayTxID = *gatewayTxID
	}
	if paymentMethod != nil {
		inv.PaymentMethod = *paymentMethod
	}
	return &inv, nil
}
