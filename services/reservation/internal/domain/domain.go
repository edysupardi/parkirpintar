package domain

import (
	"context"
	"time"
)

type VehicleType string
type AssignmentMode string
type ReservationStatus string

const (
	VehicleTypeCar        VehicleType = "CAR"
	VehicleTypeMotorcycle VehicleType = "MOTORCYCLE"

	AssignmentModeSystem   AssignmentMode = "SYSTEM"
	AssignmentModeSelected AssignmentMode = "USER_SELECTED"

	StatusPending   ReservationStatus = "PENDING"
	StatusConfirmed ReservationStatus = "CONFIRMED"
	StatusActive    ReservationStatus = "ACTIVE"
	StatusCompleted ReservationStatus = "COMPLETED"
	StatusCancelled ReservationStatus = "CANCELLED"
	StatusExpired   ReservationStatus = "EXPIRED"

	HoldDuration = 1 * time.Hour
)

type Spot struct {
	SpotID      string
	Floor       int32
	SpotNumber  int32
	VehicleType VehicleType
}

type Reservation struct {
	ReservationID  string
	DriverID       string
	Spot           Spot
	Status         ReservationStatus
	AssignmentMode AssignmentMode
	ConfirmedAt    time.Time
	ExpiresAt      time.Time
	CheckInAt      *time.Time
	CheckOutAt     *time.Time
	CancelledAt    *time.Time
	SessionID      string
	IdempotencyKey string
}

type Repository interface {
	InsertReservation(ctx context.Context, r Reservation) error
	GetReservation(ctx context.Context, reservationID string) (*Reservation, error)
	GetActiveReservation(ctx context.Context, driverID string) (*Reservation, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*Reservation, error)
	UpdateStatus(ctx context.Context, reservationID string, status ReservationStatus) error
	UpdateCheckIn(ctx context.Context, reservationID, sessionID string, checkInAt time.Time) error
	UpdateCheckOut(ctx context.Context, reservationID string, checkOutAt time.Time) error
	UpdateCancelled(ctx context.Context, reservationID string, cancelledAt time.Time) error
	FindAvailableSpot(ctx context.Context, vehicleType VehicleType) (*Spot, error)
	GetSpot(ctx context.Context, spotID string) (*Spot, error)
	CountAvailable(ctx context.Context, vehicleType VehicleType) (int32, error)
	ListAvailableSpots(ctx context.Context, vehicleType VehicleType, floor int32) ([]Spot, error)
	ListExpiredReservations(ctx context.Context, before time.Time) ([]Reservation, error)
}

type Locker interface {
	Acquire(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, key, value string) error
}

type EventPublisher interface {
	PublishReservationConfirmed(ctx context.Context, r Reservation) error
	PublishReservationExpired(ctx context.Context, r Reservation) error
	PublishReservationCancelled(ctx context.Context, r Reservation) error
	PublishCheckInDetected(ctx context.Context, r Reservation) error
	PublishCheckOutCompleted(ctx context.Context, r Reservation) error
}
