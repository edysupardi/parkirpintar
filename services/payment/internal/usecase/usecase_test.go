package usecase_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/payment/internal/domain"
	"github.com/edysupardi/parkirpintar/services/payment/internal/usecase"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockRepo struct {
	tx             *domain.Transaction
	txList         []domain.Transaction
	insertErr      error
	getErr         error
	updateErr      error
	idempotencyTx  *domain.Transaction
}

func (m *mockRepo) InsertTransaction(_ context.Context, _ domain.Transaction) error {
	return m.insertErr
}
func (m *mockRepo) GetTransaction(_ context.Context, _ string) (*domain.Transaction, error) {
	return m.tx, m.getErr
}
func (m *mockRepo) GetByGatewayTxID(_ context.Context, _ string) (*domain.Transaction, error) {
	return m.tx, m.getErr
}
func (m *mockRepo) GetByIdempotencyKey(_ context.Context, _ string) (*domain.Transaction, error) {
	return m.idempotencyTx, m.getErr
}
func (m *mockRepo) UpdateStatus(_ context.Context, _ string, _ domain.TransactionStatus, _ *time.Time) error {
	return m.updateErr
}
func (m *mockRepo) ListByInvoice(_ context.Context, _ string) ([]domain.Transaction, error) {
	return m.txList, m.getErr
}

type mockGateway struct {
	qrString   string
	paymentURL string
	vaNumber   string
	expiredAt  time.Time
	signature  string
	err        error
}

func (m *mockGateway) CreateQRIS(_ context.Context, _ string, _ int64, _ domain.CustomerInfo) (string, string, time.Time, error) {
	return m.qrString, m.paymentURL, m.expiredAt, m.err
}
func (m *mockGateway) CreateVA(_ context.Context, _, _ string, _ int64, _ domain.CustomerInfo) (string, time.Time, error) {
	return m.vaNumber, m.expiredAt, m.err
}
func (m *mockGateway) VerifyWebhookSignature(_, _, _, _ string) string {
	return m.signature
}

// --- Helpers ---

func setupRedis(t *testing.T) *redis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	return redis.NewClient(&redis.Options{Addr: mr.Addr()})
}

func newTestUsecase(repo domain.Repository, gw domain.PaymentGateway, rdb *redis.Client) *usecase.PaymentUsecase {
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	store := idempotency.New(rdb)
	return usecase.New(repo, gw, store, "server-key-123", log)
}

func testCustomer() domain.CustomerInfo {
	return domain.CustomerInfo{Name: "Edy", Email: "edy@test.com", Phone: "08123"}
}

// --- CreateTransaction Tests ---

func TestCreateTransaction_QRIS_HappyPath(t *testing.T) {
	rdb := setupRedis(t)
	exp := time.Now().Add(15 * time.Minute)
	gw := &mockGateway{qrString: "qr-data", paymentURL: "https://pay.test/qr", expiredAt: exp}
	repo := &mockRepo{}

	uc := newTestUsecase(repo, gw, rdb)
	tx, err := uc.CreateTransaction(context.Background(), "inv-1", "drv-1", "pay-key-1", "QRIS", 15_000, testCustomer())

	require.NoError(t, err)
	assert.Equal(t, "inv-1", tx.InvoiceID)
	assert.Equal(t, domain.StatusPending, tx.Status)
	assert.Equal(t, "qr-data", tx.QRString)
	assert.Equal(t, "https://pay.test/qr", tx.PaymentURL)
	assert.Equal(t, int64(15_000), tx.Amount)
}

func TestCreateTransaction_VA_HappyPath(t *testing.T) {
	rdb := setupRedis(t)
	exp := time.Now().Add(24 * time.Hour)
	gw := &mockGateway{vaNumber: "8001234567890", expiredAt: exp}
	repo := &mockRepo{}

	uc := newTestUsecase(repo, gw, rdb)
	tx, err := uc.CreateTransaction(context.Background(), "inv-1", "drv-1", "pay-key-2", "VA_BCA", 30_000, testCustomer())

	require.NoError(t, err)
	assert.Equal(t, "8001234567890", tx.VANumber)
	assert.Equal(t, "BCA", tx.VABank)
	assert.Equal(t, int64(30_000), tx.Amount)
}

func TestCreateTransaction_UnsupportedMethod(t *testing.T) {
	rdb := setupRedis(t)
	gw := &mockGateway{}
	repo := &mockRepo{}

	uc := newTestUsecase(repo, gw, rdb)
	_, err := uc.CreateTransaction(context.Background(), "inv-1", "drv-1", "pay-key-3", "BITCOIN", 15_000, testCustomer())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported payment method")
}

func TestCreateTransaction_GatewayError(t *testing.T) {
	rdb := setupRedis(t)
	gw := &mockGateway{err: errors.New("gateway timeout")}
	repo := &mockRepo{}

	uc := newTestUsecase(repo, gw, rdb)
	_, err := uc.CreateTransaction(context.Background(), "inv-1", "drv-1", "pay-key-4", "QRIS", 15_000, testCustomer())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "create QRIS")
}

func TestCreateTransaction_IdempotencyHit(t *testing.T) {
	rdb := setupRedis(t)
	cached := &domain.Transaction{TransactionID: "tx-cached", InvoiceID: "inv-1"}
	gw := &mockGateway{qrString: "qr", paymentURL: "url", expiredAt: time.Now().Add(time.Hour)}
	repo := &mockRepo{idempotencyTx: cached}

	uc := newTestUsecase(repo, gw, rdb)

	// first call
	_, err := uc.CreateTransaction(context.Background(), "inv-1", "drv-1", "dup-pay", "QRIS", 15_000, testCustomer())
	require.NoError(t, err)

	// second call — returns cached
	tx, err := uc.CreateTransaction(context.Background(), "inv-1", "drv-1", "dup-pay", "QRIS", 15_000, testCustomer())
	require.NoError(t, err)
	assert.Equal(t, "tx-cached", tx.TransactionID)
}

func TestCreateTransaction_DBInsertError(t *testing.T) {
	rdb := setupRedis(t)
	gw := &mockGateway{qrString: "qr", paymentURL: "url", expiredAt: time.Now().Add(time.Hour)}
	repo := &mockRepo{insertErr: errors.New("db down")}

	uc := newTestUsecase(repo, gw, rdb)
	_, err := uc.CreateTransaction(context.Background(), "inv-1", "drv-1", "pay-key-5", "QRIS", 15_000, testCustomer())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "insert transaction")
}

// --- GetTransactionStatus Tests ---

func TestGetTransactionStatus_Found(t *testing.T) {
	rdb := setupRedis(t)
	tx := &domain.Transaction{TransactionID: "tx-1", Status: domain.StatusSettled}
	repo := &mockRepo{tx: tx}

	uc := newTestUsecase(repo, &mockGateway{}, rdb)
	result, err := uc.GetTransactionStatus(context.Background(), "tx-1")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusSettled, result.Status)
}

func TestGetTransactionStatus_NotFound(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{getErr: errors.New("not found")}

	uc := newTestUsecase(repo, &mockGateway{}, rdb)
	_, err := uc.GetTransactionStatus(context.Background(), "tx-999")

	require.Error(t, err)
}

// --- ListByInvoice Tests ---

func TestListByInvoice(t *testing.T) {
	rdb := setupRedis(t)
	txList := []domain.Transaction{
		{TransactionID: "tx-1", InvoiceID: "inv-1"},
		{TransactionID: "tx-2", InvoiceID: "inv-1"},
	}
	repo := &mockRepo{txList: txList}

	uc := newTestUsecase(repo, &mockGateway{}, rdb)
	result, err := uc.ListByInvoice(context.Background(), "inv-1")

	require.NoError(t, err)
	assert.Len(t, result, 2)
}

// --- HandleWebhook Tests ---

func makeWebhookPayload(orderID, status, fraudStatus, grossAmount, sigKey string) []byte {
	p := map[string]string{
		"order_id":           orderID,
		"status_code":        "200",
		"gross_amount":       grossAmount,
		"signature_key":      sigKey,
		"transaction_status": status,
		"fraud_status":       fraudStatus,
	}
	b, _ := json.Marshal(p)
	return b
}

func TestHandleWebhook_Settlement_Success(t *testing.T) {
	rdb := setupRedis(t)
	tx := &domain.Transaction{TransactionID: "tx-1", GatewayTxID: "PP-abc12345", Status: domain.StatusPending}
	repo := &mockRepo{tx: tx}
	gw := &mockGateway{signature: "valid-sig"}

	uc := newTestUsecase(repo, gw, rdb)
	payload := makeWebhookPayload("PP-abc12345", "settlement", "accept", "15000.00", "valid-sig")

	processed, isDup, txID, status, err := uc.HandleWebhook(context.Background(), payload, "")

	require.NoError(t, err)
	assert.True(t, processed)
	assert.False(t, isDup)
	assert.Equal(t, "tx-1", txID)
	assert.Equal(t, domain.StatusSettled, status)
}

func TestHandleWebhook_InvalidSignature(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{}
	gw := &mockGateway{signature: "expected-sig"}

	uc := newTestUsecase(repo, gw, rdb)
	payload := makeWebhookPayload("PP-abc12345", "settlement", "accept", "15000.00", "wrong-sig")

	_, _, _, _, err := uc.HandleWebhook(context.Background(), payload, "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature")
}

func TestHandleWebhook_Idempotent_Duplicate(t *testing.T) {
	rdb := setupRedis(t)
	tx := &domain.Transaction{TransactionID: "tx-1", GatewayTxID: "PP-abc12345", Status: domain.StatusSettled}
	repo := &mockRepo{tx: tx}
	gw := &mockGateway{signature: "valid-sig"}

	uc := newTestUsecase(repo, gw, rdb)
	payload := makeWebhookPayload("PP-abc12345", "settlement", "accept", "15000.00", "valid-sig")

	// first call
	_, _, _, _, err := uc.HandleWebhook(context.Background(), payload, "")
	require.NoError(t, err)

	// second call — duplicate
	processed, isDup, _, _, err := uc.HandleWebhook(context.Background(), payload, "")
	require.NoError(t, err)
	assert.True(t, processed)
	assert.True(t, isDup)
}

func TestHandleWebhook_Failure_Status(t *testing.T) {
	rdb := setupRedis(t)
	tx := &domain.Transaction{TransactionID: "tx-1", GatewayTxID: "PP-abc12345", Status: domain.StatusPending}
	repo := &mockRepo{tx: tx}
	gw := &mockGateway{signature: "valid-sig"}

	uc := newTestUsecase(repo, gw, rdb)
	payload := makeWebhookPayload("PP-abc12345", "deny", "", "15000.00", "valid-sig")

	processed, isDup, _, status, err := uc.HandleWebhook(context.Background(), payload, "")

	require.NoError(t, err)
	assert.True(t, processed)
	assert.False(t, isDup)
	assert.Equal(t, domain.StatusFailed, status)
}

func TestHandleWebhook_Expire_Status(t *testing.T) {
	rdb := setupRedis(t)
	tx := &domain.Transaction{TransactionID: "tx-1", GatewayTxID: "PP-abc12345", Status: domain.StatusPending}
	repo := &mockRepo{tx: tx}
	gw := &mockGateway{signature: "valid-sig"}

	uc := newTestUsecase(repo, gw, rdb)
	payload := makeWebhookPayload("PP-abc12345", "expire", "", "15000.00", "valid-sig")

	_, _, _, status, err := uc.HandleWebhook(context.Background(), payload, "")

	require.NoError(t, err)
	assert.Equal(t, domain.StatusExpired, status)
}

func TestHandleWebhook_InvalidPayload(t *testing.T) {
	rdb := setupRedis(t)
	repo := &mockRepo{}
	gw := &mockGateway{}

	uc := newTestUsecase(repo, gw, rdb)
	_, _, _, _, err := uc.HandleWebhook(context.Background(), []byte("not json"), "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid payload")
}
