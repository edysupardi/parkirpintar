package subscriber_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	notificationv1 "github.com/edysupardi/parkirpintar/gen/notification/v1"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/pkg/mq"
	"github.com/edysupardi/parkirpintar/services/notification/internal/subscriber"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSender struct {
	called    bool
	lastReq   *notificationv1.SendNotificationRequest
	resp      *notificationv1.SendNotificationResponse
	err       error
}

func (m *mockSender) SendNotification(_ context.Context, req *notificationv1.SendNotificationRequest) (*notificationv1.SendNotificationResponse, error) {
	m.called = true
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	if m.resp != nil {
		return m.resp, nil
	}
	return &notificationv1.SendNotificationResponse{Success: true, NotificationId: "notif-001"}, nil
}

func newMsg(t *testing.T, event string, payload any) mq.Message {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return mq.Message{Event: event, Timestamp: time.Now(), Payload: raw}
}

func TestNotificationSubscriber_Handle_AllEvents(t *testing.T) {
	cases := []struct {
		event      string
		templateID notificationv1.TemplateID
	}{
		{mq.EventReservationConfirmed, notificationv1.TemplateID_TEMPLATE_ID_RESERVATION_CONFIRMED},
		{mq.EventReservationExpired, notificationv1.TemplateID_TEMPLATE_ID_RESERVATION_EXPIRED},
		{mq.EventReservationCancelled, notificationv1.TemplateID_TEMPLATE_ID_RESERVATION_CANCELLED},
		{mq.EventCheckInDetected, notificationv1.TemplateID_TEMPLATE_ID_CHECK_IN_SUCCESS},
		{mq.EventCheckOutCompleted, notificationv1.TemplateID_TEMPLATE_ID_CHECK_OUT_SUCCESS},
	}

	for _, tc := range cases {
		t.Run(tc.event, func(t *testing.T) {
			sender := &mockSender{}
			log := logger.New(logger.Config{Service: "test", Level: "error"})
			sub := subscriber.New(sender, log)

			msg := newMsg(t, tc.event, map[string]any{
				"reservation_id": "res-001",
				"driver_id":      "drv-001",
				"floor":          1,
				"spot_number":    5,
			})

			err := sub.Handle(context.Background(), msg)
			require.NoError(t, err)
			assert.True(t, sender.called)
			assert.Equal(t, "drv-001", sender.lastReq.DriverId)
			assert.Equal(t, tc.templateID, sender.lastReq.TemplateId)
			assert.Equal(t, notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_PUSH, sender.lastReq.Channel)
		})
	}
}

func TestNotificationSubscriber_Handle_InvalidPayload(t *testing.T) {
	sender := &mockSender{}
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	sub := subscriber.New(sender, log)

	msg := mq.Message{
		Event:     mq.EventReservationConfirmed,
		Timestamp: time.Now(),
		Payload:   []byte("not-json"),
	}

	err := sub.Handle(context.Background(), msg)
	require.Error(t, err)
	assert.False(t, sender.called)
}

func TestNotificationSubscriber_Handle_SenderError(t *testing.T) {
	sender := &mockSender{err: errors.New("handler error")}
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	sub := subscriber.New(sender, log)

	msg := newMsg(t, mq.EventReservationConfirmed, map[string]any{
		"reservation_id": "res-001",
		"driver_id":      "drv-001",
	})

	err := sub.Handle(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "handler error")
}

func TestNotificationSubscriber_Handle_SendFailed_NoError(t *testing.T) {
	sender := &mockSender{
		resp: &notificationv1.SendNotificationResponse{Success: false, ErrorMessage: "fcm down"},
	}
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	sub := subscriber.New(sender, log)

	msg := newMsg(t, mq.EventCheckInDetected, map[string]any{
		"reservation_id": "res-001",
		"driver_id":      "drv-001",
		"floor":          2,
		"spot_number":    10,
	})

	// send failure is logged but not returned as error (best-effort)
	err := sub.Handle(context.Background(), msg)
	require.NoError(t, err)
}
