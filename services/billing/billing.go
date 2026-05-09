package billing

import (
	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/billing/internal/domain"
	"github.com/edysupardi/parkirpintar/services/billing/internal/repository"
	"github.com/edysupardi/parkirpintar/services/billing/internal/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type InvoiceStatus = domain.InvoiceStatus

const (
	InvoiceStatusPendingPayment = domain.InvoiceStatusPendingPayment
	InvoiceStatusPaid           = domain.InvoiceStatusPaid
)

type Invoice = domain.Invoice
type Usecase = usecase.BillingUsecase

func NewUsecase(pool *pgxpool.Pool, rdb *redis.Client, log logger.Logger) *usecase.BillingUsecase {
	repo := repository.New(pool)
	idem := idempotency.New(rdb)
	return usecase.New(repo, idem, log)
}
