package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/domain"
	"github.com/google/uuid"
)

type ReservationUsecase struct {
	repo        domain.Repository
	locker      domain.Locker
	publisher   domain.EventPublisher
	idempotency *idempotency.Store
	log         logger.Logger
}

func New(
	repo domain.Repository,
	locker domain.Locker,
	publisher domain.EventPublisher,
	idempotency *idempotency.Store,
	log logger.Logger,
) *ReservationUsecase {
	return &ReservationUsecase{
		repo:        repo,
		locker:      locker,
		publisher:   publisher,
		idempotency: idempotency,
		log:         log,
	}
}

func spotLockKey(spotID string) string {
	return fmt.Sprintf("spot:%s:lock", spotID)
}

func (uc *ReservationUsecase) CreateReservation(ctx context.Context, driverID, idempotencyKey string, vehicleType domain.VehicleType, mode domain.AssignmentMode, spotID string) (*domain.Reservation, error) {
	// idempotency check
	cached, hit, err := uc.idempotency.Check(ctx, "idempotency:"+idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("idempotency check: %w", err)
	}
	if hit {
		return uc.repo.GetByIdempotencyKey(ctx, idempotencyKey)
	}
	_ = cached

	// find spot
	var spot *domain.Spot
	if mode == domain.AssignmentModeSelected {
		spot, err = uc.repo.GetSpot(ctx, spotID)
		if err != nil {
			return nil, fmt.Errorf("get spot: %w", err)
		}
	} else {
		spot, err = uc.repo.FindAvailableSpot(ctx, vehicleType)
		if err != nil {
			return nil, fmt.Errorf("find available spot: %w", err)
		}
	}
	if spot == nil {
		return nil, fmt.Errorf("no available spot")
	}

	reservationID := uuid.New().String()
	lockKey := spotLockKey(spot.SpotID)

	ok, err := uc.locker.Acquire(ctx, lockKey, reservationID, domain.HoldDuration)
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("spot unavailable")
	}

	now := time.Now()
	r := domain.Reservation{
		ReservationID:  reservationID,
		DriverID:       driverID,
		Spot:           *spot,
		Status:         domain.StatusConfirmed,
		AssignmentMode: mode,
		ConfirmedAt:    now,
		ExpiresAt:      now.Add(domain.HoldDuration),
		IdempotencyKey: idempotencyKey,
	}

	if err := uc.repo.InsertReservation(ctx, r); err != nil {
		_ = uc.locker.Release(ctx, lockKey, reservationID)
		return nil, fmt.Errorf("insert reservation: %w", err)
	}

	if err := uc.idempotency.Save(ctx, "idempotency:"+idempotencyKey, reservationID, idempotency.DefaultTTL); err != nil {
		uc.log.Warn(ctx).Err(err).Msg("failed to save idempotency key")
	}

	if err := uc.publisher.PublishReservationConfirmed(ctx, r); err != nil {
		uc.log.Warn(ctx).Err(err).Msg("failed to publish ReservationConfirmed")
	}

	return &r, nil
}

func (uc *ReservationUsecase) CancelReservation(ctx context.Context, reservationID, driverID string) (*domain.Reservation, int64, error) {
	r, err := uc.repo.GetReservation(ctx, reservationID)
	if err != nil {
		return nil, 0, fmt.Errorf("get reservation: %w", err)
	}
	if r.DriverID != driverID {
		return nil, 0, fmt.Errorf("reservation not owned by driver")
	}
	if r.Status != domain.StatusConfirmed {
		return nil, 0, fmt.Errorf("reservation cannot be cancelled in status %s", r.Status)
	}

	now := time.Now()
	var fee int64
	if now.Sub(r.ConfirmedAt) <= 2*time.Minute {
		fee = 0
	} else {
		fee = 5_000
	}

	if err := uc.repo.UpdateCancelled(ctx, reservationID, now); err != nil {
		return nil, 0, fmt.Errorf("update cancelled: %w", err)
	}
	_ = uc.locker.Release(ctx, spotLockKey(r.Spot.SpotID), reservationID)

	r.Status = domain.StatusCancelled
	r.CancelledAt = &now

	if err := uc.publisher.PublishReservationCancelled(ctx, *r); err != nil {
		uc.log.Warn(ctx).Err(err).Msg("failed to publish ReservationCancelled")
	}

	return r, fee, nil
}

func (uc *ReservationUsecase) CheckIn(ctx context.Context, reservationID, driverID string) (*domain.Reservation, error) {
	r, err := uc.repo.GetReservation(ctx, reservationID)
	if err != nil {
		return nil, fmt.Errorf("get reservation: %w", err)
	}
	if r.DriverID != driverID {
		return nil, fmt.Errorf("reservation not owned by driver")
	}
	if r.Status != domain.StatusConfirmed {
		return nil, fmt.Errorf("cannot check in: status is %s", r.Status)
	}
	if time.Now().After(r.ExpiresAt) {
		return nil, fmt.Errorf("reservation expired")
	}

	sessionID := uuid.New().String()
	now := time.Now()

	if err := uc.repo.UpdateCheckIn(ctx, reservationID, sessionID, now); err != nil {
		return nil, fmt.Errorf("update check-in: %w", err)
	}

	r.Status = domain.StatusActive
	r.SessionID = sessionID
	r.CheckInAt = &now

	if err := uc.publisher.PublishCheckInDetected(ctx, *r); err != nil {
		uc.log.Warn(ctx).Err(err).Msg("failed to publish CheckInDetected")
	}

	return r, nil
}

func (uc *ReservationUsecase) CheckOut(ctx context.Context, reservationID, driverID, idempotencyKey string) (*domain.Reservation, error) {
	_, hit, err := uc.idempotency.Check(ctx, "idempotency:"+idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("idempotency check: %w", err)
	}
	if hit {
		return uc.repo.GetReservation(ctx, reservationID)
	}

	r, err := uc.repo.GetReservation(ctx, reservationID)
	if err != nil {
		return nil, fmt.Errorf("get reservation: %w", err)
	}
	if r.DriverID != driverID {
		return nil, fmt.Errorf("reservation not owned by driver")
	}
	if r.Status != domain.StatusActive {
		return nil, fmt.Errorf("cannot check out: status is %s", r.Status)
	}

	now := time.Now()
	if err := uc.repo.UpdateCheckOut(ctx, reservationID, now); err != nil {
		return nil, fmt.Errorf("update check-out: %w", err)
	}

	_ = uc.locker.Release(ctx, spotLockKey(r.Spot.SpotID), reservationID)

	if err := uc.idempotency.Save(ctx, "idempotency:"+idempotencyKey, reservationID, idempotency.DefaultTTL); err != nil {
		uc.log.Warn(ctx).Err(err).Msg("failed to save idempotency key")
	}

	r.Status = domain.StatusCompleted
	r.CheckOutAt = &now

	if err := uc.publisher.PublishCheckOutCompleted(ctx, *r); err != nil {
		uc.log.Warn(ctx).Err(err).Msg("failed to publish CheckOutCompleted")
	}

	return r, nil
}

func (uc *ReservationUsecase) GetReservation(ctx context.Context, reservationID string) (*domain.Reservation, error) {
	return uc.repo.GetReservation(ctx, reservationID)
}

func (uc *ReservationUsecase) GetActiveReservation(ctx context.Context, driverID string) (*domain.Reservation, error) {
	return uc.repo.GetActiveReservation(ctx, driverID)
}

func (uc *ReservationUsecase) GetAvailability(ctx context.Context, vehicleType domain.VehicleType) (total, available, occupied, reserved int32, err error) {
	available, err = uc.repo.CountAvailable(ctx, vehicleType)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if vehicleType == domain.VehicleTypeCar {
		total = 150 // 5 floors × 30 cars
	} else {
		total = 250 // 5 floors × 50 motorcycles
	}
	occupied = total - available
	return total, available, occupied, 0, nil
}

func (uc *ReservationUsecase) ListAvailableSpots(ctx context.Context, vehicleType domain.VehicleType, floor int32) ([]domain.Spot, error) {
	return uc.repo.ListAvailableSpots(ctx, vehicleType, floor)
}

// ExpireReservations is called by the background job to expire no-show reservations.
func (uc *ReservationUsecase) ExpireReservations(ctx context.Context) error {
	expired, err := uc.repo.ListExpiredReservations(ctx, time.Now())
	if err != nil {
		return fmt.Errorf("list expired: %w", err)
	}
	for _, r := range expired {
		if err := uc.repo.UpdateStatus(ctx, r.ReservationID, domain.StatusExpired); err != nil {
			uc.log.Error(ctx).Err(err).Str("reservation_id", r.ReservationID).Msg("failed to expire reservation")
			continue
		}
		_ = uc.locker.Release(ctx, spotLockKey(r.Spot.SpotID), r.ReservationID)
		if err := uc.publisher.PublishReservationExpired(ctx, r); err != nil {
			uc.log.Warn(ctx).Err(err).Msg("failed to publish ReservationExpired")
		}
	}
	return nil
}
