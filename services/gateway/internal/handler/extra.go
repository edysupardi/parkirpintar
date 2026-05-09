package handler

import (
	"context"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/auth"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ExtraHandler struct {
	db        *pgxpool.Pool
	validator *auth.Validator
	serverKey string
}

func NewExtra(db *pgxpool.Pool, validator *auth.Validator, serverKey string) *ExtraHandler {
	return &ExtraHandler{db: db, validator: validator, serverKey: serverKey}
}

// CreateBookingFeeInvoice creates a booking_fee invoice and auto-settles payment.
// Called by gateway during reservation flow.
func (h *ExtraHandler) CreateBookingFeeInvoice(ctx context.Context, reservationID, driverID, idempotencyKey string) (invoiceID string, err error) {
	invoiceID = uuid.New().String()
	txID := uuid.New().String()
	orderID := "PP-" + txID[:8]
	now := time.Now()

	_, err = h.db.Exec(ctx, `
		INSERT INTO invoices (id, type, reservation_id, driver_id, booking_fee, total_amount, status, idempotency_key)
		VALUES ($1, 'booking_fee', $2, $3, 5000, 5000, 'paid', $4)`,
		invoiceID, reservationID, driverID, idempotencyKey)
	if err != nil {
		return "", fmt.Errorf("insert booking fee invoice: %w", err)
	}

	_, err = h.db.Exec(ctx, `
		INSERT INTO transactions (id, invoice_id, driver_id, gateway_tx_id, payment_method, status, amount, idempotency_key, paid_at)
		VALUES ($1, $2, $3, $4, 'QRIS', 'settled', 5000, $5, $6)`,
		txID, invoiceID, driverID, orderID, "booking-"+idempotencyKey, now)
	if err != nil {
		return "", fmt.Errorf("insert booking fee transaction: %w", err)
	}

	return invoiceID, nil
}

// GET /v1/reservations/history
func (h *ExtraHandler) ReservationHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing user id")
		return
	}

	rows, err := h.db.Query(context.Background(), `
		SELECT r.id, r.status, r.confirmed_at, r.check_in_at, r.check_out_at, r.cancelled_at,
		       s.floor, s.number, s.vehicle_type
		FROM reservations r
		JOIN spots s ON s.id = r.spot_id
		WHERE r.driver_id = $1
		ORDER BY r.confirmed_at DESC
		LIMIT 20
	`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query history")
		return
	}
	defer rows.Close()

	var history []map[string]any
	for rows.Next() {
		var id, status, vt string
		var confirmedAt time.Time
		var checkInAt, checkOutAt, cancelledAt *time.Time
		var floor, number int32

		if err := rows.Scan(&id, &status, &confirmedAt, &checkInAt, &checkOutAt, &cancelledAt, &floor, &number, &vt); err != nil {
			continue
		}
		entry := map[string]any{
			"reservation_id": id,
			"status":         status,
			"confirmed_at":   confirmedAt,
			"spot": map[string]any{
				"floor":        floor,
				"number":       number,
				"vehicle_type": vt,
			},
		}
		if checkInAt != nil {
			entry["check_in_at"] = checkInAt
		}
		if checkOutAt != nil {
			entry["check_out_at"] = checkOutAt
		}
		if cancelledAt != nil {
			entry["cancelled_at"] = cancelledAt
		}
		history = append(history, entry)
	}
	if history == nil {
		history = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"reservations": history})
}

// GET /v1/parking/spots?vehicle_type=car&floor=1
func (h *ExtraHandler) ListAvailableSpots(w http.ResponseWriter, r *http.Request) {
	vt := r.URL.Query().Get("vehicle_type")
	if vt == "" {
		vt = "car"
	}
	// normalize from proto enum to db value
	vt = strings.ToLower(strings.TrimPrefix(vt, "VEHICLE_TYPE_"))

	floor := r.URL.Query().Get("floor")

	query := `SELECT id, floor, number, vehicle_type FROM spots WHERE vehicle_type = $1 AND status = 'available'`
	args := []any{vt}
	if floor != "" {
		query += ` AND floor = $2`
		args = append(args, floor)
	}
	query += ` ORDER BY floor ASC, number ASC`

	rows, err := h.db.Query(context.Background(), query, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query spots")
		return
	}
	defer rows.Close()

	var spots []map[string]any
	for rows.Next() {
		var id, vehicleType string
		var f, num int32
		if err := rows.Scan(&id, &f, &num, &vehicleType); err != nil {
			continue
		}
		spots = append(spots, map[string]any{
			"spot_id":      id,
			"floor":        f,
			"number":       num,
			"vehicle_type": vehicleType,
		})
	}
	if spots == nil {
		spots = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"spots": spots, "count": len(spots)})
}

// POST /v1/payments/simulate-settle
func (h *ExtraHandler) SimulateSettle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TransactionID string `json:"transaction_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TransactionID == "" {
		writeError(w, http.StatusBadRequest, "transaction_id is required")
		return
	}

	// get transaction details
	var gatewayTxID string
	var amount int64
	err := h.db.QueryRow(context.Background(),
		`SELECT gateway_tx_id, amount FROM transactions WHERE id = $1`, req.TransactionID,
	).Scan(&gatewayTxID, &amount)
	if err != nil {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}

	// build webhook payload matching Midtrans format
	statusCode := "200"
	grossAmount := fmt.Sprintf("%d.00", amount)
	sig := sha512Sum(gatewayTxID + statusCode + grossAmount + h.serverKey)

	payload := map[string]string{
		"order_id":           gatewayTxID,
		"status_code":       statusCode,
		"gross_amount":      grossAmount,
		"signature_key":     sig,
		"transaction_status": "settlement",
		"fraud_status":      "accept",
	}

	// update transaction directly
	now := time.Now()
	_, err = h.db.Exec(context.Background(),
		`UPDATE transactions SET status = 'settled', paid_at = $1 WHERE id = $2`,
		now, req.TransactionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update transaction")
		return
	}

	// also mark invoice as paid
	var invoiceID string
	_ = h.db.QueryRow(context.Background(),
		`SELECT invoice_id FROM transactions WHERE id = $1`, req.TransactionID,
	).Scan(&invoiceID)
	if invoiceID != "" {
		_, _ = h.db.Exec(context.Background(),
			`UPDATE invoices SET status = 'paid', gateway_tx_id = $1, payment_method = 'QRIS', paid_at = $2 WHERE id = $3`,
			gatewayTxID, now, invoiceID)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"message":        "settlement simulated",
		"transaction_id": req.TransactionID,
		"invoice_id":     invoiceID,
		"status":         "settled",
		"webhook_payload": payload,
	})
}

func sha512Sum(s string) string {
	h := sha512.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))
}
