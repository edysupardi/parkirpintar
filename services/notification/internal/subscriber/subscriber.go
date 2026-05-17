package subscriber

import (
	"context"
	"encoding/json"
	"fmt"

	notificationv1 "github.com/edysupardi/parkirpintar/gen/notification/v1"
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

type NotificationSender interface {
	SendNotification(ctx context.Context, req *notificationv1.SendNotificationRequest) (*notificationv1.SendNotificationResponse, error)
}

type NotificationSubscriber struct {
	sender NotificationSender
	log    logger.Logger
}

func New(sender NotificationSender, log logger.Logger) *NotificationSubscriber {
	return &NotificationSubscriber{sender: sender, log: log}
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

	templateID := eventToTemplate(msg.Event)

	resp, err := s.sender.SendNotification(ctx, &notificationv1.SendNotificationRequest{
		DriverId:   evt.DriverID,
		Channel:    notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_PUSH,
		TemplateId: templateID,
		Data: map[string]string{
			"event":          msg.Event,
			"reservation_id": evt.ReservationID,
			"spot":           fmt.Sprintf("%d", evt.SpotNumber),
			"floor":          fmt.Sprintf("%d", evt.Floor),
		},
	})
	if err != nil {
		s.log.Error(ctx).Err(err).Str("event", msg.Event).Msg("failed to send notification via handler")
		return err
	}

	if !resp.Success {
		s.log.Warn(ctx).Str("event", msg.Event).Str("error", resp.ErrorMessage).Msg("notification send failed")
	} else {
		s.log.Info(ctx).
			Str("event", msg.Event).
			Str("driver_id", evt.DriverID).
			Str("notification_id", resp.NotificationId).
			Msg("notification sent and persisted")
	}

	return nil
}

func eventToTemplate(event string) notificationv1.TemplateID {
	switch event {
	case mq.EventReservationConfirmed:
		return notificationv1.TemplateID_TEMPLATE_ID_RESERVATION_CONFIRMED
	case mq.EventReservationExpired:
		return notificationv1.TemplateID_TEMPLATE_ID_RESERVATION_EXPIRED
	case mq.EventReservationCancelled:
		return notificationv1.TemplateID_TEMPLATE_ID_RESERVATION_CANCELLED
	case mq.EventCheckInDetected:
		return notificationv1.TemplateID_TEMPLATE_ID_CHECK_IN_SUCCESS
	case mq.EventCheckOutCompleted:
		return notificationv1.TemplateID_TEMPLATE_ID_CHECK_OUT_SUCCESS
	default:
		return notificationv1.TemplateID_TEMPLATE_ID_UNSPECIFIED
	}
}
