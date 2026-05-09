package reservation

import (
	"context"

	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/lock"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/domain"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/repository"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/usecase"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type VehicleType = domain.VehicleType
type AssignmentMode = domain.AssignmentMode
type ReservationStatus = domain.ReservationStatus
type Reservation = domain.Reservation
type Usecase = usecase.ReservationUsecase

const (
	VehicleTypeCar        = domain.VehicleTypeCar
	VehicleTypeMotorcycle = domain.VehicleTypeMotorcycle
	AssignmentModeSystem  = domain.AssignmentModeSystem
	AssignmentModeSelected = domain.AssignmentModeSelected
	StatusConfirmed       = domain.StatusConfirmed
	StatusActive          = domain.StatusActive
	StatusCompleted       = domain.StatusCompleted
	StatusCancelled       = domain.StatusCancelled
	StatusExpired         = domain.StatusExpired
)

type NoopPublisher struct{}

func (n *NoopPublisher) PublishReservationConfirmed(_ context.Context, _ domain.Reservation) error { return nil }
func (n *NoopPublisher) PublishReservationExpired(_ context.Context, _ domain.Reservation) error  { return nil }
func (n *NoopPublisher) PublishReservationCancelled(_ context.Context, _ domain.Reservation) error { return nil }
func (n *NoopPublisher) PublishCheckInDetected(_ context.Context, _ domain.Reservation) error     { return nil }
func (n *NoopPublisher) PublishCheckOutCompleted(_ context.Context, _ domain.Reservation) error   { return nil }

func NewUsecase(pool *pgxpool.Pool, rdb *redis.Client, log logger.Logger) *usecase.ReservationUsecase {
	repo := repository.New(pool)
	locker := lock.New(rdb)
	idem := idempotency.New(rdb)
	return usecase.New(repo, locker, &NoopPublisher{}, idem, log)
}
