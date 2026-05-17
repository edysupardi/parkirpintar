package subscriber

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/pkg/mq"
	"github.com/edysupardi/parkirpintar/services/billing/internal/domain"
	"github.com/google/uuid"
)

type reservationEvent struct {
	ReservationID string `json:"reservation_id"`
	DriverID      string `json:"driver_id"`
	SessionID     string `json:"session_id"`
}

type invoiceGenerator interface {
	GenerateInvoice(ctx context.Context, sessionID, reservationID, driverID, idempotencyKey string, checkIn, checkOut time.Time) (*domain.Invoice, error)
}

type BillingSubscriber struct {
	uc  invoiceGenerator
	log logger.Logger
}

func New(uc invoiceGenerator, log logger.Logger) *BillingSubscriber {
	return &BillingSubscriber{uc: uc, log: log}
}

func (s *BillingSubscriber) Register(consumer *mq.Consumer) error {
	return consumer.Subscribe(
		"billing.checkout",
		[]string{mq.EventCheckOutCompleted},
		s.HandleCheckOut,
	)
}

func (s *BillingSubscriber) HandleCheckOut(ctx context.Context, msg mq.Message) error {
	var evt reservationEvent
	if err := json.Unmarshal(msg.Payload, &evt); err != nil {
		return fmt.Errorf("unmarshal checkout event: %w", err)
	}

	if evt.SessionID == "" {
		s.log.Warn(ctx).Str("reservation_id", evt.ReservationID).Msg("checkout event missing session_id, skipping")
		return nil
	}

	checkOut := msg.Timestamp
	checkIn := checkOut.Add(-1 * time.Hour)

	_, err := s.uc.GenerateInvoice(ctx,
		evt.SessionID,
		evt.ReservationID,
		evt.DriverID,
		uuid.New().String(),
		checkIn,
		checkOut,
	)
	if err != nil {
		s.log.Error(ctx).Err(err).Str("reservation_id", evt.ReservationID).Msg("failed to generate invoice from MQ event")
		return err
	}

	s.log.Info(ctx).Str("reservation_id", evt.ReservationID).Str("session_id", evt.SessionID).Msg("invoice generated from checkout event")
	return nil
}
