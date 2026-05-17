package publisher

import (
	"context"
	"encoding/json"

	"github.com/edysupardi/parkirpintar/pkg/mq"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/domain"
)

type ReservationEvent struct {
	ReservationID string `json:"reservation_id"`
	DriverID      string `json:"driver_id"`
	SpotID        string `json:"spot_id"`
	Floor         int32  `json:"floor"`
	SpotNumber    int32  `json:"spot_number"`
	VehicleType   string `json:"vehicle_type"`
	SessionID     string `json:"session_id,omitempty"`
	Status        string `json:"status"`
}

func fromDomain(r domain.Reservation) ReservationEvent {
	return ReservationEvent{
		ReservationID: r.ReservationID,
		DriverID:      r.DriverID,
		SpotID:        r.Spot.SpotID,
		Floor:         r.Spot.Floor,
		SpotNumber:    r.Spot.SpotNumber,
		VehicleType:   string(r.Spot.VehicleType),
		SessionID:     r.SessionID,
		Status:        string(r.Status),
	}
}

type MQPublisher struct {
	pub *mq.Publisher
}

func New(pub *mq.Publisher) *MQPublisher {
	return &MQPublisher{pub: pub}
}

func (p *MQPublisher) PublishReservationConfirmed(ctx context.Context, r domain.Reservation) error {
	return p.publish(ctx, mq.EventReservationConfirmed, r)
}

func (p *MQPublisher) PublishReservationExpired(ctx context.Context, r domain.Reservation) error {
	return p.publish(ctx, mq.EventReservationExpired, r)
}

func (p *MQPublisher) PublishReservationCancelled(ctx context.Context, r domain.Reservation) error {
	return p.publish(ctx, mq.EventReservationCancelled, r)
}

func (p *MQPublisher) PublishCheckInDetected(ctx context.Context, r domain.Reservation) error {
	return p.publish(ctx, mq.EventCheckInDetected, r)
}

func (p *MQPublisher) PublishCheckOutCompleted(ctx context.Context, r domain.Reservation) error {
	return p.publish(ctx, mq.EventCheckOutCompleted, r)
}

func (p *MQPublisher) publish(ctx context.Context, event string, r domain.Reservation) error {
	payload, err := json.Marshal(fromDomain(r))
	if err != nil {
		return err
	}
	return p.pub.Publish(ctx, event, json.RawMessage(payload))
}
