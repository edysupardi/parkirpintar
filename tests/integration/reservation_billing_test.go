//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/billing"
	"github.com/edysupardi/parkirpintar/services/reservation"
	"github.com/edysupardi/parkirpintar/tests/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func uid() string { return uuid.New().String() }

func TestIntegration_ReservationToBilling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	ctx := context.Background()

	db, err := testutil.NewTestDB(ctx)
	require.NoError(t, err)
	defer db.Close(ctx)

	rds, err := testutil.NewTestRedis()
	require.NoError(t, err)
	defer rds.Close()

	log := logger.New(logger.Config{Service: "integration-test", Level: "error"})
	reservationUC := reservation.NewUsecase(db.Pool, rds.Client, log)
	billingUsecase := billing.NewUsecase(db.Pool, rds.Client, log)

	driverID := uid()

	// Step 1: Create reservation (returns pending)
	res, err := reservationUC.CreateReservation(ctx, driverID, uid(), reservation.VehicleTypeCar, reservation.AssignmentModeSystem, "")
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusPending, res.Status)
	assert.NotEmpty(t, res.Spot.SpotID)

	// Step 1b: Confirm reservation (simulates payment confirmed)
	res, err = reservationUC.ConfirmReservation(ctx, res.ReservationID)
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusConfirmed, res.Status)

	// Step 2: Check-in
	res, err = reservationUC.CheckIn(ctx, res.ReservationID, driverID)
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusActive, res.Status)
	assert.NotEmpty(t, res.SessionID)

	// Step 3: Check-out
	res, err = reservationUC.CheckOut(ctx, res.ReservationID, driverID, uid())
	require.NoError(t, err)
	assert.Equal(t, reservation.StatusCompleted, res.Status)
	assert.NotNil(t, res.CheckOutAt)

	// Step 4: Generate invoice
	checkIn := res.CheckOutAt.Add(-1*time.Hour - 1*time.Minute)
	checkOut := *res.CheckOutAt
	inv, err := billingUsecase.GenerateInvoice(ctx, res.SessionID, res.ReservationID, driverID, uid(), checkIn, checkOut)
	require.NoError(t, err)
	assert.Equal(t, billing.InvoiceStatusPendingPayment, inv.Status)
	assert.Equal(t, int64(5_000), inv.BookingFee)
	assert.Equal(t, int64(10_000), inv.TotalAmount) // parking only, booking fee charged at reserve

	// Step 5: Mark paid
	now := time.Now()
	paidInv, err := billingUsecase.MarkInvoicePaid(ctx, inv.InvoiceID, "gw-tx-001", "QRIS", now)
	require.NoError(t, err)
	assert.Equal(t, billing.InvoiceStatusPaid, paidInv.Status)
	assert.NotNil(t, paidInv.PaidAt)

	// Step 6: Idempotency — same key returns same invoice
	inv2, err := billingUsecase.GenerateInvoice(ctx, res.SessionID, res.ReservationID, driverID, inv.IdempotencyKey, checkIn, checkOut)
	require.NoError(t, err)
	assert.Equal(t, inv.InvoiceID, inv2.InvoiceID)
}
