package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/pkg/pricing"
	"github.com/edysupardi/parkirpintar/services/billing/internal/domain"
	"github.com/edysupardi/parkirpintar/services/billing/internal/usecase"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockRepo struct {
	invoice          *domain.Invoice
	insertErr        error
	getErr           error
	markPaidErr      error
	idempotencyInv   *domain.Invoice
}

func (m *mockRepo) InsertInvoice(_ context.Context, _ domain.Invoice) error {
	return m.insertErr
}
func (m *mockRepo) GetInvoice(_ context.Context, _ string) (*domain.Invoice, error) {
	return m.invoice, m.getErr
}
func (m *mockRepo) GetInvoiceBySession(_ context.Context, _ string) (*domain.Invoice, error) {
	return m.invoice, m.getErr
}
func (m *mockRepo) GetByIdempotencyKey(_ context.Context, _ string) (*domain.Invoice, error) {
	return m.idempotencyInv, m.getErr
}
func (m *mockRepo) MarkPaid(_ context.Context, _, _, _ string, _ time.Time) error {
	return m.markPaidErr
}

// --- Helpers ---

func setupRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func newTestUsecase(repo domain.Repository, rdb *redis.Client) *usecase.BillingUsecase {
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	store := idempotency.New(rdb)
	return usecase.New(repo, store, log)
}

// --- GenerateInvoice Tests ---

func TestGenerateInvoice_HappyPath(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{}

	uc := newTestUsecase(repo, rdb)
	checkIn := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	checkOut := time.Date(2026, 5, 20, 12, 30, 0, 0, time.UTC)

	inv, err := uc.GenerateInvoice(context.Background(), "sess-1", "res-1", "drv-1", "key-1", checkIn, checkOut)

	require.NoError(t, err)
	assert.Equal(t, domain.InvoiceTypeParkingSession, inv.Type)
	assert.Equal(t, domain.InvoiceStatusPendingPayment, inv.Status)
	assert.Equal(t, "drv-1", inv.DriverID)
	assert.Equal(t, int32(3), inv.BilledHours) // ceil(2.5h) = 3
	assert.Equal(t, int64(15_000), inv.ParkingFee)
	assert.Equal(t, int64(150), inv.DurationMins)
}

func TestGenerateInvoice_Overnight(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{}

	uc := newTestUsecase(repo, rdb)
	// 23:00 WIB = 16:00 UTC, 01:00 WIB = 18:00 UTC
	wib := time.FixedZone("WIB", 7*3600)
	checkIn := time.Date(2026, 5, 20, 23, 0, 0, 0, wib)
	checkOut := time.Date(2026, 5, 21, 1, 0, 0, 0, wib)

	inv, err := uc.GenerateInvoice(context.Background(), "sess-2", "res-2", "drv-1", "key-2", checkIn, checkOut)

	require.NoError(t, err)
	assert.True(t, inv.IsOvernight)
	assert.Equal(t, int64(20_000), inv.OvernightFee)
	assert.Equal(t, int64(10_000), inv.ParkingFee) // 2h × 5000
}

func TestGenerateInvoice_IdempotencyHit(t *testing.T) {
	rdb := setupRedis(t)
	cached := &domain.Invoice{InvoiceID: "inv-cached", DriverID: "drv-1"}
	repo := &mockRepo{idempotencyInv: cached}

	uc := newTestUsecase(repo, rdb)
	checkIn := time.Now().Add(-1 * time.Hour)
	checkOut := time.Now()

	// first call
	_, err := uc.GenerateInvoice(context.Background(), "sess-1", "res-1", "drv-1", "dup-key", checkIn, checkOut)
	require.NoError(t, err)

	// second call — returns cached
	inv, err := uc.GenerateInvoice(context.Background(), "sess-1", "res-1", "drv-1", "dup-key", checkIn, checkOut)
	require.NoError(t, err)
	assert.Equal(t, "inv-cached", inv.InvoiceID)
}

func TestGenerateInvoice_DBError(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{insertErr: errors.New("db down")}

	uc := newTestUsecase(repo, rdb)
	checkIn := time.Now().Add(-1 * time.Hour)
	checkOut := time.Now()

	_, err := uc.GenerateInvoice(context.Background(), "sess-1", "res-1", "drv-1", "key-3", checkIn, checkOut)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "insert invoice")
}

// --- GenerateBookingFeeInvoice Tests ---

func TestGenerateBookingFeeInvoice_HappyPath(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{}

	uc := newTestUsecase(repo, rdb)
	inv, err := uc.GenerateBookingFeeInvoice(context.Background(), "res-1", "drv-1", "bf-key-1")

	require.NoError(t, err)
	assert.Equal(t, domain.InvoiceTypeBookingFee, inv.Type)
	assert.Equal(t, int64(pricing.BookingFee), inv.BookingFee)
	assert.Equal(t, int64(pricing.BookingFee), inv.TotalAmount)
	assert.Equal(t, domain.InvoiceStatusPendingPayment, inv.Status)
}

func TestGenerateBookingFeeInvoice_IdempotencyHit(t *testing.T) {
	rdb := setupRedis(t)
	cached := &domain.Invoice{InvoiceID: "inv-bf-cached"}
	repo := &mockRepo{idempotencyInv: cached}

	uc := newTestUsecase(repo, rdb)

	_, err := uc.GenerateBookingFeeInvoice(context.Background(), "res-1", "drv-1", "bf-dup")
	require.NoError(t, err)

	inv, err := uc.GenerateBookingFeeInvoice(context.Background(), "res-1", "drv-1", "bf-dup")
	require.NoError(t, err)
	assert.Equal(t, "inv-bf-cached", inv.InvoiceID)
}

func TestGenerateBookingFeeInvoice_DBError(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{insertErr: errors.New("db down")}

	uc := newTestUsecase(repo, rdb)
	_, err := uc.GenerateBookingFeeInvoice(context.Background(), "res-1", "drv-1", "bf-key-2")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "insert booking fee invoice")
}

// --- GetInvoice Tests ---

func TestGetInvoice_Found(t *testing.T) {
	rdb := setupRedis(t)
	inv := &domain.Invoice{InvoiceID: "inv-1", DriverID: "drv-1"}
	repo := &mockRepo{invoice: inv}

	uc := newTestUsecase(repo, rdb)
	result, err := uc.GetInvoice(context.Background(), "inv-1")

	require.NoError(t, err)
	assert.Equal(t, "inv-1", result.InvoiceID)
}

func TestGetInvoice_NotFound(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{getErr: errors.New("not found")}

	uc := newTestUsecase(repo, rdb)
	_, err := uc.GetInvoice(context.Background(), "inv-999")

	require.Error(t, err)
}

// --- GetInvoiceBySession Tests ---

func TestGetInvoiceBySession_Found(t *testing.T) {
	rdb := setupRedis(t)
	inv := &domain.Invoice{InvoiceID: "inv-1", SessionID: "sess-1"}
	repo := &mockRepo{invoice: inv}

	uc := newTestUsecase(repo, rdb)
	result, err := uc.GetInvoiceBySession(context.Background(), "sess-1")

	require.NoError(t, err)
	assert.Equal(t, "sess-1", result.SessionID)
}

// --- PreviewBilling Tests ---

func TestPreviewBilling(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{}

	uc := newTestUsecase(repo, rdb)
	checkIn := time.Now().Add(-2 * time.Hour)
	result := uc.PreviewBilling(context.Background(), checkIn)

	assert.GreaterOrEqual(t, result.BilledHours, int32(2))
	assert.GreaterOrEqual(t, result.ParkingFee, int64(10_000))
}

// --- CalculateCancellationFee Tests ---

func TestCalculateCancellationFee_Under2Min(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{}

	uc := newTestUsecase(repo, rdb)
	confirmed := time.Now().Add(-1 * time.Minute)
	cancelled := time.Now()

	fee, reason := uc.CalculateCancellationFee(context.Background(), confirmed, cancelled, false)

	assert.Equal(t, int64(0), fee)
	assert.Contains(t, reason, "within 2 minutes")
}

func TestCalculateCancellationFee_Over2Min(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{}

	uc := newTestUsecase(repo, rdb)
	confirmed := time.Now().Add(-5 * time.Minute)
	cancelled := time.Now()

	fee, reason := uc.CalculateCancellationFee(context.Background(), confirmed, cancelled, false)

	assert.Equal(t, int64(5_000), fee)
	assert.Contains(t, reason, "5.000")
}

func TestCalculateCancellationFee_NoShow(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{}

	uc := newTestUsecase(repo, rdb)
	confirmed := time.Now().Add(-2 * time.Hour)
	cancelled := time.Now()

	fee, reason := uc.CalculateCancellationFee(context.Background(), confirmed, cancelled, true)

	assert.Equal(t, int64(0), fee)
	assert.Contains(t, reason, "No-show")
}

// --- MarkInvoicePaid Tests ---

func TestMarkInvoicePaid_HappyPath(t *testing.T) {
	rdb := setupRedis(t)
	paidAt := time.Now()
	inv := &domain.Invoice{InvoiceID: "inv-1", Status: domain.InvoiceStatusPaid, GatewayTxID: "tx-1"}
	repo := &mockRepo{invoice: inv}

	uc := newTestUsecase(repo, rdb)
	result, err := uc.MarkInvoicePaid(context.Background(), "inv-1", "tx-1", "qris", paidAt)

	require.NoError(t, err)
	assert.Equal(t, domain.InvoiceStatusPaid, result.Status)
	assert.Equal(t, "tx-1", result.GatewayTxID)
}

func TestMarkInvoicePaid_DBError(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{markPaidErr: errors.New("db error")}

	uc := newTestUsecase(repo, rdb)
	_, err := uc.MarkInvoicePaid(context.Background(), "inv-1", "tx-1", "qris", time.Now())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mark paid")
}
