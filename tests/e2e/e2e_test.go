//go:build e2e

package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/billing"
	"github.com/edysupardi/parkirpintar/services/payment"
	"github.com/edysupardi/parkirpintar/services/reservation"
	"github.com/edysupardi/parkirpintar/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func uid() string { return uuid.New().String() }

var (
	testDB  *testutil.TestDB
	testRds *testutil.TestRedis
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error

	testDB, err = testutil.NewTestDB(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup db: %v\n", err)
		os.Exit(1)
	}

	testRds, err = testutil.NewTestRedis()
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup redis: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	testRds.Close()
	testDB.Close(ctx)
	os.Exit(code)
}

func newLog() logger.Logger {
	return logger.New(logger.Config{Service: "e2e", Level: "error"})
}

func newReservationUC() *reservation.Usecase {
	return reservation.NewUsecase(testDB.Pool, testRds.Client, newLog())
}

func newBillingUC() *billing.Usecase {
	return billing.NewUsecase(testDB.Pool, testRds.Client, newLog())
}

func newPaymentUC(gw *payment.MockGateway) *payment.Usecase {
	return payment.NewUsecase(testDB.Pool, testRds.Client, gw, "server-key-test", newLog())
}

// helper: full reservation flow (reserve → confirm → check-in → check-out), returns completed reservation
func doFullReservationFlow(t *testing.T, ctx context.Context, ruc *reservation.Usecase) *reservation.Reservation {
	t.Helper()
	driverID := uid()

	res, err := ruc.CreateReservation(ctx, driverID, uid(), reservation.VehicleTypeCar, reservation.AssignmentModeSystem, "")
	require.NoError(t, err)

	res, err = ruc.ConfirmReservation(ctx, res.ReservationID)
	require.NoError(t, err)

	res, err = ruc.CheckIn(ctx, res.ReservationID, driverID)
	require.NoError(t, err)

	res, err = ruc.CheckOut(ctx, res.ReservationID, driverID, uid())
	require.NoError(t, err)

	return res
}

// ── E2E-01: Happy path ─────────────────────────────────────────────────────

func TestE2E_01_HappyPath(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()
	buc := newBillingUC()
	gw := &payment.MockGateway{}
	puc := newPaymentUC(gw)

	driverID := uid()

	res, err := ruc.CreateReservation(ctx, driverID, uid(), reservation.VehicleTypeCar, reservation.AssignmentModeSystem, "")
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusPending, res.Status)

	res, err = ruc.ConfirmReservation(ctx, res.ReservationID)
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusConfirmed, res.Status)

	res, err = ruc.CheckIn(ctx, res.ReservationID, driverID)
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusActive, res.Status)

	res, err = ruc.CheckOut(ctx, res.ReservationID, driverID, uid())
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusCompleted, res.Status)

	checkIn := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)  // 10:00 — no overnight
	checkOut := time.Date(2026, 5, 5, 11, 1, 0, 0, time.UTC) // 11:01 — 2h ceil
	inv, err := buc.GenerateInvoice(ctx, res.SessionID, res.ReservationID, driverID, uid(), checkIn, checkOut)
	require.NoError(t, err)
	assert.Equal(t, billing.InvoiceStatusPendingPayment, inv.Status)
	assert.Equal(t, int32(2), inv.BilledHours)
	assert.Equal(t, int64(10_000), inv.TotalAmount) // parking only, booking fee charged at reserve

	customer := payment.CustomerInfo{Name: "Test", Email: "t@t.com", Phone: "08123"}
	tx, err := puc.CreateTransaction(ctx, inv.InvoiceID, driverID, uid(), "QRIS", inv.TotalAmount, customer)
	require.NoError(t, err)
	assert.Equal(t, payment.StatusPending, tx.Status)
	assert.NotEmpty(t, tx.QRString)
}

// ── E2E-02: Double-book prevention ─────────────────────────────────────────

func TestE2E_02_DoubleBookPrevention(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()

	res1, err := ruc.CreateReservation(ctx, uid(), uid(), reservation.VehicleTypeCar, reservation.AssignmentModeSystem, "")
	require.NoError(t, err)

	_, err = ruc.CreateReservation(ctx, uid(), uid(), reservation.VehicleTypeCar, reservation.AssignmentModeSelected, res1.Spot.SpotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spot unavailable")
}

// ── E2E-03: User-selected spot contention ───────────────────────────────────

func TestE2E_03_UserSelectedSpotContention(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()

	res1, err := ruc.CreateReservation(ctx, uid(), uid(), reservation.VehicleTypeMotorcycle, reservation.AssignmentModeSystem, "")
	require.NoError(t, err)

	_, err = ruc.CreateReservation(ctx, uid(), uid(), reservation.VehicleTypeMotorcycle, reservation.AssignmentModeSelected, res1.Spot.SpotID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spot unavailable")
}

// ── E2E-04: Reservation expiry (no-show) ───────────────────────────────────

func TestE2E_04_ReservationExpiry_NoShow(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()
	buc := newBillingUC()

	driverID := uid()
	res, err := ruc.CreateReservation(ctx, driverID, uid(), reservation.VehicleTypeCar, reservation.AssignmentModeSystem, "")
	require.NoError(t, err)

	_, err = testDB.Pool.Exec(ctx, `UPDATE reservations SET expires_at = $1 WHERE id = $2`,
		time.Now().Add(-1*time.Minute), res.ReservationID)
	require.NoError(t, err)

	err = ruc.ExpireReservations(ctx)
	require.NoError(t, err)

	expired, err := ruc.GetReservation(ctx, res.ReservationID)
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusExpired, expired.Status)

	fee, reason := buc.CalculateCancellationFee(ctx, res.ConfirmedAt, time.Now(), true)
	assert.Equal(t, int64(0), fee)
	assert.Contains(t, reason, "No-show")
}

// ── E2E-05: Cancellation < 2 min ───────────────────────────────────────────

func TestE2E_05_CancellationUnder2Min(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()

	driverID := uid()
	res, err := ruc.CreateReservation(ctx, driverID, uid(), reservation.VehicleTypeCar, reservation.AssignmentModeSystem, "")
	require.NoError(t, err)

	cancelled, fee, err := ruc.CancelReservation(ctx, res.ReservationID, driverID)
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusCancelled, cancelled.Status)
	assert.Equal(t, int64(0), fee)
}

// ── E2E-06: Cancellation > 2 min ───────────────────────────────────────────

func TestE2E_06_CancellationOver2Min(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()

	driverID := uid()
	res, err := ruc.CreateReservation(ctx, driverID, uid(), reservation.VehicleTypeCar, reservation.AssignmentModeSystem, "")
	require.NoError(t, err)

	_, err = testDB.Pool.Exec(ctx, `UPDATE reservations SET confirmed_at = $1 WHERE id = $2`,
		time.Now().Add(-3*time.Minute), res.ReservationID)
	require.NoError(t, err)

	cancelled, fee, err := ruc.CancelReservation(ctx, res.ReservationID, driverID)
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusCancelled, cancelled.Status)
	assert.Equal(t, int64(5_000), fee)
}

// ── E2E-07: Extended stay (overstay) — no penalty ───────────────────────────

func TestE2E_07_ExtendedStay_NoOverstayPenalty(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()
	buc := newBillingUC()

	driverID := uid()
	res, err := ruc.CreateReservation(ctx, driverID, uid(), reservation.VehicleTypeCar, reservation.AssignmentModeSystem, "")
	require.NoError(t, err)

	res, err = ruc.ConfirmReservation(ctx, res.ReservationID)
	require.NoError(t, err)

	res, err = ruc.CheckIn(ctx, res.ReservationID, driverID)
	require.NoError(t, err)

	res, err = ruc.CheckOut(ctx, res.ReservationID, driverID, uid())
	require.NoError(t, err)

	// simulate 3 hour stay (fixed times, no overnight)
	checkIn := time.Date(2026, 5, 5, 10, 0, 0, 0, time.UTC)
	checkOut := time.Date(2026, 5, 5, 13, 0, 0, 0, time.UTC)
	inv, err := buc.GenerateInvoice(ctx, res.SessionID, res.ReservationID, driverID, uid(), checkIn, checkOut)
	require.NoError(t, err)

	assert.Equal(t, int64(3), int64(inv.BilledHours))
	assert.Equal(t, int64(15_000), inv.ParkingFee)
	assert.Equal(t, int64(5_000), inv.BookingFee)
	assert.Equal(t, int64(15_000), inv.TotalAmount)
}

// ── E2E-08: Overnight fee ───────────────────────────────────────────────────

func TestE2E_08_OvernightFee(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()
	buc := newBillingUC()

	// create a real reservation for FK
	res := doFullReservationFlow(t, ctx, ruc)

	wib := time.FixedZone("WIB", 7*60*60)
	checkIn := time.Date(2026, 5, 5, 23, 0, 0, 0, wib)
	checkOut := time.Date(2026, 5, 6, 1, 0, 0, 0, wib)

	inv, err := buc.GenerateInvoice(ctx, res.SessionID, res.ReservationID, res.DriverID, uid(), checkIn, checkOut)
	require.NoError(t, err)

	assert.True(t, inv.IsOvernight)
	assert.Equal(t, int64(20_000), inv.OvernightFee)
	assert.Equal(t, int32(2), inv.BilledHours)
	assert.Equal(t, int64(30_000), inv.TotalAmount)
}

// ── E2E-09: Payment QRIS success ───────────────────────────────────────────

func TestE2E_09_PaymentQRIS_Success(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()
	buc := newBillingUC()
	gw := &payment.MockGateway{}
	puc := newPaymentUC(gw)

	res := doFullReservationFlow(t, ctx, ruc)

	inv, err := buc.GenerateInvoice(ctx, res.SessionID, res.ReservationID, res.DriverID, uid(),
		time.Now().Add(-1*time.Hour), time.Now())
	require.NoError(t, err)

	customer := payment.CustomerInfo{Name: "Test", Email: "t@t.com", Phone: "08123"}
	tx, err := puc.CreateTransaction(ctx, inv.InvoiceID, res.DriverID, uid(), "QRIS", inv.TotalAmount, customer)
	require.NoError(t, err)
	assert.Equal(t, payment.StatusPending, tx.Status)
	assert.NotEmpty(t, tx.QRString)

	sig := gw.VerifyWebhookSignature(tx.GatewayTxID, "200", fmt.Sprintf("%d.00", inv.TotalAmount), "server-key-test")
	payload, _ := json.Marshal(map[string]string{
		"order_id":            tx.GatewayTxID,
		"status_code":        "200",
		"gross_amount":       fmt.Sprintf("%d.00", inv.TotalAmount),
		"signature_key":      sig,
		"transaction_status": "settlement",
		"fraud_status":       "accept",
	})

	processed, isDup, txID, status, err := puc.HandleWebhook(ctx, payload, sig)
	require.NoError(t, err)
	assert.True(t, processed)
	assert.False(t, isDup)
	assert.Equal(t, tx.TransactionID, txID)
	assert.Equal(t, payment.StatusSettled, status)
}

// ── E2E-10: Payment QRIS failure ────────────────────────────────────────────

func TestE2E_10_PaymentQRIS_Failure(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()
	buc := newBillingUC()
	gw := &payment.MockGateway{}
	puc := newPaymentUC(gw)

	res := doFullReservationFlow(t, ctx, ruc)

	inv, err := buc.GenerateInvoice(ctx, res.SessionID, res.ReservationID, res.DriverID, uid(),
		time.Now().Add(-1*time.Hour), time.Now())
	require.NoError(t, err)

	customer := payment.CustomerInfo{Name: "Test", Email: "t@t.com", Phone: "08123"}
	tx, err := puc.CreateTransaction(ctx, inv.InvoiceID, res.DriverID, uid(), "QRIS", inv.TotalAmount, customer)
	require.NoError(t, err)

	sig := gw.VerifyWebhookSignature(tx.GatewayTxID, "202", fmt.Sprintf("%d.00", inv.TotalAmount), "server-key-test")
	payload, _ := json.Marshal(map[string]string{
		"order_id":            tx.GatewayTxID,
		"status_code":        "202",
		"gross_amount":       fmt.Sprintf("%d.00", inv.TotalAmount),
		"signature_key":      sig,
		"transaction_status": "deny",
		"fraud_status":       "",
	})

	processed, isDup, _, status, err := puc.HandleWebhook(ctx, payload, sig)
	require.NoError(t, err)
	assert.True(t, processed)
	assert.False(t, isDup)
	assert.Equal(t, payment.StatusFailed, status)

	updated, err := puc.GetTransactionStatus(ctx, tx.TransactionID)
	require.NoError(t, err)
	assert.Equal(t, payment.StatusFailed, updated.Status)
}

// ── E2E-11: Payment Virtual Account ─────────────────────────────────────────

func TestE2E_11_PaymentVirtualAccount(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()
	buc := newBillingUC()
	gw := &payment.MockGateway{}
	puc := newPaymentUC(gw)

	res := doFullReservationFlow(t, ctx, ruc)

	inv, err := buc.GenerateInvoice(ctx, res.SessionID, res.ReservationID, res.DriverID, uid(),
		time.Now().Add(-1*time.Hour), time.Now())
	require.NoError(t, err)

	customer := payment.CustomerInfo{Name: "Test", Email: "t@t.com", Phone: "08123"}
	tx, err := puc.CreateTransaction(ctx, inv.InvoiceID, res.DriverID, uid(), "VA_BCA", inv.TotalAmount, customer)
	require.NoError(t, err)
	assert.Equal(t, payment.StatusPending, tx.Status)
	assert.NotEmpty(t, tx.VANumber)
	assert.Equal(t, "BCA", tx.VABank)

	sig := gw.VerifyWebhookSignature(tx.GatewayTxID, "200", fmt.Sprintf("%d.00", inv.TotalAmount), "server-key-test")
	payload, _ := json.Marshal(map[string]string{
		"order_id":            tx.GatewayTxID,
		"status_code":        "200",
		"gross_amount":       fmt.Sprintf("%d.00", inv.TotalAmount),
		"signature_key":      sig,
		"transaction_status": "settlement",
		"fraud_status":       "accept",
	})

	processed, isDup, _, status, err := puc.HandleWebhook(ctx, payload, sig)
	require.NoError(t, err)
	assert.True(t, processed)
	assert.False(t, isDup)
	assert.Equal(t, payment.StatusSettled, status)
}

// ── E2E-12: Duplicate webhook — idempotent ──────────────────────────────────

func TestE2E_12_DuplicateWebhook_Idempotent(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()
	buc := newBillingUC()
	gw := &payment.MockGateway{}
	puc := newPaymentUC(gw)

	res := doFullReservationFlow(t, ctx, ruc)

	inv, err := buc.GenerateInvoice(ctx, res.SessionID, res.ReservationID, res.DriverID, uid(),
		time.Now().Add(-1*time.Hour), time.Now())
	require.NoError(t, err)

	customer := payment.CustomerInfo{Name: "Test", Email: "t@t.com", Phone: "08123"}
	tx, err := puc.CreateTransaction(ctx, inv.InvoiceID, res.DriverID, uid(), "QRIS", inv.TotalAmount, customer)
	require.NoError(t, err)

	sig := gw.VerifyWebhookSignature(tx.GatewayTxID, "200", fmt.Sprintf("%d.00", inv.TotalAmount), "server-key-test")
	payload, _ := json.Marshal(map[string]string{
		"order_id":            tx.GatewayTxID,
		"status_code":        "200",
		"gross_amount":       fmt.Sprintf("%d.00", inv.TotalAmount),
		"signature_key":      sig,
		"transaction_status": "settlement",
		"fraud_status":       "accept",
	})

	// first webhook
	processed1, isDup1, _, status1, err := puc.HandleWebhook(ctx, payload, sig)
	require.NoError(t, err)
	assert.True(t, processed1)
	assert.False(t, isDup1)
	assert.Equal(t, payment.StatusSettled, status1)

	// duplicate webhook — should be idempotent
	processed2, isDup2, _, status2, err := puc.HandleWebhook(ctx, payload, sig)
	require.NoError(t, err)
	assert.True(t, processed2)
	assert.True(t, isDup2)
	assert.Equal(t, payment.StatusSettled, status2)
}

// ── E2E-13: Wrong-spot check-in — check-in rejected ────────────────────────

func TestE2E_13_WrongSpotCheckIn(t *testing.T) {
	testRds.FlushAll()
	ctx := context.Background()
	ruc := newReservationUC()

	driverID := uid()

	res, err := ruc.CreateReservation(ctx, driverID, uid(), reservation.VehicleTypeCar, reservation.AssignmentModeSystem, "")
	require.NoError(t, err)

	res, err = ruc.ConfirmReservation(ctx, res.ReservationID)
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusConfirmed, res.Status)

	// Attempt check-in with wrong reservation ID (simulates wrong spot at usecase level)
	// Gateway validates actual_spot_id vs booked spot_id and returns error before calling CheckIn.
	// At usecase level: CheckIn only accepts the correct reservation — wrong spot means
	// driver tries to check in to a reservation that doesn't belong to them.
	_, err = ruc.CheckIn(ctx, res.ReservationID, "wrong-driver-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not owned by driver")

	// Correct driver can still check in to their own spot
	res, err = ruc.CheckIn(ctx, res.ReservationID, driverID)
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusActive, res.Status)
}
