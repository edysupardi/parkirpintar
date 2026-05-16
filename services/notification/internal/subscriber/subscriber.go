package subscriber

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/pkg/mq"
)

type reservationEvent struct {
	ReservationID string `json:"reservation_id"`
	DriverID      string `json:"driver_id"`
	SpotID        string `json:"spot_id"`
	Floor         int32  `json:"floor"`
	SpotNumber    int32  `json:"spot_number"`
	SessionID     string `json:"session_id,omitempty"`
	Status        string `json:"status"`
}

type PushProvider interface {
	Send(ctx context.Context, driverID, title, body string, data map[string]string) error
}

type NotificationSubscriber struct {
	push PushProvider
	log  logger.Logger
}

func New(push PushProvider, log logger.Logger) *NotificationSubscriber {
	return &NotificationSubscriber{push: push, log: log}
}

func (s *NotificationSubscriber) Register(consumer *mq.Consumer) error {
	return consumer.Subscribe(
		"notification.all",
		[]string{
			mq.EventReservationConfirmed,
			mq.EventReservationExpired,
			mq.EventReservationCancelled,
			mq.EventCheckInDetected,
			mq.EventCheckOutCompleted,
		},
		s.Handle,
	)
}

func (s *NotificationSubscriber) Handle(ctx context.Context, msg mq.Message) error {
	var evt reservationEvent
	if err := json.Unmarshal(msg.Payload, &evt); err != nil {
		return fmt.Errorf("unmarshal notification event: %w", err)
	}

	title, body := s.buildContent(msg.Event, evt)
	if err := s.push.Send(ctx, evt.DriverID, title, body, map[string]string{
		"event":          msg.Event,
		"reservation_id": evt.ReservationID,
	}); err != nil {
		s.log.Warn(ctx).Err(err).Str("event", msg.Event).Msg("failed to send push notification")
	}

	s.log.Info(ctx).
		Str("event", msg.Event).
		Str("driver_id", evt.DriverID).
		Str("reservation_id", evt.ReservationID).
		Msg("notification sent")

	return nil
}

func (s *NotificationSubscriber) BuildContent(event string, evt reservationEvent) (title, body string) {
	return s.buildContent(event, evt)
}

func (s *NotificationSubscriber) buildContent(event string, evt reservationEvent) (title, body string) {
	switch event {
	case mq.EventReservationConfirmed:
		return "Reservasi Dikonfirmasi", fmt.Sprintf("Spot lantai %d nomor %d sudah dipesan. Silakan check-in dalam 1 jam.", evt.Floor, evt.SpotNumber)
	case mq.EventReservationExpired:
		return "Reservasi Kadaluarsa", "Reservasi Anda telah kadaluarsa karena tidak check-in dalam 1 jam."
	case mq.EventReservationCancelled:
		return "Reservasi Dibatalkan", "Reservasi Anda telah dibatalkan."
	case mq.EventCheckInDetected:
		return "Check-in Berhasil", fmt.Sprintf("Selamat datang! Anda sudah check-in di spot lantai %d nomor %d.", evt.Floor, evt.SpotNumber)
	case mq.EventCheckOutCompleted:
		return "Check-out Berhasil", "Terima kasih telah menggunakan ParkirPintar. Invoice sedang diproses."
	default:
		return "Notifikasi ParkirPintar", event
	}
}
