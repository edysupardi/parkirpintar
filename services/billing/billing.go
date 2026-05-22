package billing

import (
	billingv1 "github.com/edysupardi/parkirpintar/gen/billing/v1"
	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/billing/internal/domain"
	"github.com/edysupardi/parkirpintar/services/billing/internal/handler"
	"github.com/edysupardi/parkirpintar/services/billing/internal/repository"
	"github.com/edysupardi/parkirpintar/services/billing/internal/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
)

type InvoiceStatus = domain.InvoiceStatus
type InvoiceType = domain.InvoiceType

const (
	InvoiceStatusPendingPayment = domain.InvoiceStatusPendingPayment
	InvoiceStatusPaid           = domain.InvoiceStatusPaid
	InvoiceTypeBookingFee       = domain.InvoiceTypeBookingFee
	InvoiceTypeParkingSession   = domain.InvoiceTypeParkingSession
)

type Invoice = domain.Invoice
type Usecase = usecase.BillingUsecase

func NewUsecase(pool *pgxpool.Pool, rdb *redis.Client, log logger.Logger) *usecase.BillingUsecase {
	repo := repository.New(pool)
	idem := idempotency.New(rdb)
	return usecase.New(repo, idem, log)
}

func RegisterServer(srv *grpc.Server, pool *pgxpool.Pool, rdb *redis.Client, log logger.Logger) {
	uc := NewUsecase(pool, rdb, log)
	billingv1.RegisterBillingServiceServer(srv, handler.New(uc))
}
