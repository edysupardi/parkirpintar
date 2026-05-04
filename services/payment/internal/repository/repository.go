package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/edysupardi/parkirpintar/services/payment/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) InsertTransaction(ctx context.Context, tx domain.Transaction) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO transactions
			(id, invoice_id, driver_id, gateway_tx_id, payment_method, status,
			 amount, payment_url, qr_string, va_number, va_bank, idempotency_key, expired_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		tx.TransactionID, tx.InvoiceID, tx.DriverID, tx.GatewayTxID, tx.PaymentMethod,
		string(tx.Status), tx.Amount, tx.PaymentURL, tx.QRString, tx.VANumber, tx.VABank,
		tx.IdempotencyKey, tx.ExpiredAt,
	)
	return err
}

func (r *PostgresRepository) GetTransaction(ctx context.Context, transactionID string) (*domain.Transaction, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, invoice_id, driver_id, gateway_tx_id, payment_method, status,
		       amount, payment_url, qr_string, va_number, va_bank, idempotency_key,
		       created_at, paid_at, expired_at
		FROM transactions WHERE id = $1`, transactionID)
	return scanTransaction(row)
}

func (r *PostgresRepository) GetByGatewayTxID(ctx context.Context, gatewayTxID string) (*domain.Transaction, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, invoice_id, driver_id, gateway_tx_id, payment_method, status,
		       amount, payment_url, qr_string, va_number, va_bank, idempotency_key,
		       created_at, paid_at, expired_at
		FROM transactions WHERE gateway_tx_id = $1`, gatewayTxID)
	return scanTransaction(row)
}

func (r *PostgresRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Transaction, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, invoice_id, driver_id, gateway_tx_id, payment_method, status,
		       amount, payment_url, qr_string, va_number, va_bank, idempotency_key,
		       created_at, paid_at, expired_at
		FROM transactions WHERE idempotency_key = $1`, key)
	return scanTransaction(row)
}

func (r *PostgresRepository) UpdateStatus(ctx context.Context, transactionID string, status domain.TransactionStatus, paidAt *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE transactions SET status = $1, paid_at = $2 WHERE id = $3`,
		string(status), paidAt, transactionID)
	return err
}

func (r *PostgresRepository) ListByInvoice(ctx context.Context, invoiceID string) ([]domain.Transaction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, invoice_id, driver_id, gateway_tx_id, payment_method, status,
		       amount, payment_url, qr_string, va_number, va_bank, idempotency_key,
		       created_at, paid_at, expired_at
		FROM transactions WHERE invoice_id = $1 ORDER BY created_at DESC`, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.Transaction
	for rows.Next() {
		tx, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *tx)
	}
	return result, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTransaction(row scanner) (*domain.Transaction, error) {
	var tx domain.Transaction
	var status string
	err := row.Scan(
		&tx.TransactionID, &tx.InvoiceID, &tx.DriverID, &tx.GatewayTxID, &tx.PaymentMethod,
		&status, &tx.Amount, &tx.PaymentURL, &tx.QRString, &tx.VANumber, &tx.VABank,
		&tx.IdempotencyKey, &tx.CreatedAt, &tx.PaidAt, &tx.ExpiredAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("transaction not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scan transaction: %w", err)
	}
	tx.Status = domain.TransactionStatus(status)
	return &tx, nil
}
