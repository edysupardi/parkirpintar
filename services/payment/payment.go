package payment

import (
	"context"
	"crypto/sha512"
	"fmt"
	"time"

	paymentv1 "github.com/edysupardi/parkirpintar/gen/payment/v1"
	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/payment/internal/domain"
	"github.com/edysupardi/parkirpintar/services/payment/internal/handler"
	"github.com/edysupardi/parkirpintar/services/payment/internal/repository"
	"github.com/edysupardi/parkirpintar/services/payment/internal/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

type TransactionStatus = domain.TransactionStatus
type Transaction = domain.Transaction
type CustomerInfo = domain.CustomerInfo
type Usecase = usecase.PaymentUsecase

const (
	StatusPending  = domain.StatusPending
	StatusSettled  = domain.StatusSettled
	StatusFailed   = domain.StatusFailed
	StatusExpired  = domain.StatusExpired
)

type MockGateway struct {
	QRISErr error
	VAErr   error
}

func (g *MockGateway) CreateQRIS(_ context.Context, orderID string, _ int64, _ domain.CustomerInfo) (string, string, time.Time, error) {
	if g.QRISErr != nil {
		return "", "", time.Time{}, g.QRISErr
	}
	return "mock-qr-string", "https://pay.mock/" + orderID, time.Now().Add(15 * time.Minute), nil
}

func (g *MockGateway) CreateVA(_ context.Context, orderID, bank string, _ int64, _ domain.CustomerInfo) (string, time.Time, error) {
	if g.VAErr != nil {
		return "", time.Time{}, g.VAErr
	}
	return "8800" + orderID[:8], time.Now().Add(24 * time.Hour), nil
}

func (g *MockGateway) VerifyWebhookSignature(orderID, statusCode, grossAmount, serverKey string) string {
	h := sha512.New()
	h.Write([]byte(orderID + statusCode + grossAmount + serverKey))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func NewUsecase(pool *pgxpool.Pool, rdb *redis.Client, gw domain.PaymentGateway, serverKey string, log logger.Logger) *usecase.PaymentUsecase {
	repo := repository.New(pool)
	idem := idempotency.New(rdb)
	return usecase.New(repo, gw, idem, serverKey, log)
}

func RegisterServer(srv *grpc.Server, pool *pgxpool.Pool, rdb *redis.Client, gw domain.PaymentGateway, serverKey string, log logger.Logger) {
	uc := NewUsecase(pool, rdb, gw, serverKey, log)
	paymentv1.RegisterPaymentServiceServer(srv, handler.New(uc))
}
