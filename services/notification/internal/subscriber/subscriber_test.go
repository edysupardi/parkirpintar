package subscriber_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/pkg/mq"
	"github.com/edysupardi/parkirpintar/services/notification/internal/subscriber"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPush struct {
	called   bool
	lastTitle string
	err      error
}

func (m *mockPush) Send(_ context.Context, _, title, _ string, _ map[string]string) error {
	m.called = true
	m.lastTitle = title
	return m.err
}

func newMsg(t *testing.T, event string, payload any) mq.Message {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return mq.Message{Event: event, Timestamp: time.Now(), Payload: raw}
}

func TestNotificationSubscriber_Handle_AllEvents(t *testing.T) {
	cases := []struct {
		event         string
		expectedTitle string
	}{
		{mq.EventReservationConfirmed, "Reservasi Dikonfirmasi"},
		{mq.EventReservationExpired, "Reservasi Kadaluarsa"},
		{mq.EventReservationCancelled, "Reservasi Dibatalkan"},
		{mq.EventCheckInDetected, "Check-in Berhasil"},
		{mq.EventCheckOutCompleted, "Check-out Berhasil"},
	}

	for _, tc := range cases {
		t.Run(tc.event, func(t *testing.T) {
			push := &mockPush{}
			log := logger.New(logger.Config{Service: "test", Level: "error"})
			sub := subscriber.New(push, log)

			msg := newMsg(t, tc.event, map[string]any{
				"reservation_id": "res-001",
				"driver_id":      "drv-001",
				"floor":          1,
				"spot_number":    5,
			})
			msg.Event = tc.event

			err := sub.Handle(context.Background(), msg)
			require.NoError(t, err)
			assert.True(t, push.called)
			assert.Equal(t, tc.expectedTitle, push.lastTitle)
		})
	}
}

func TestNotificationSubscriber_Handle_InvalidPayload(t *testing.T) {
	push := &mockPush{}
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	sub := subscriber.New(push, log)

	msg := mq.Message{
		Event:     mq.EventReservationConfirmed,
		Timestamp: time.Now(),
		Payload:   []byte("not-json"),
	}

	err := sub.Handle(context.Background(), msg)
	require.Error(t, err)
	assert.False(t, push.called)
}

func TestNotificationSubscriber_Handle_PushError_DoesNotFail(t *testing.T) {
	push := &mockPush{err: errors.New("fcm error")}
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	sub := subscriber.New(push, log)

	msg := newMsg(t, mq.EventReservationConfirmed, map[string]any{
		"reservation_id": "res-001",
		"driver_id":      "drv-001",
	})
	msg.Event = mq.EventReservationConfirmed

	// push error should not propagate — notification is best-effort
	err := sub.Handle(context.Background(), msg)
	require.NoError(t, err)
}

func TestNotificationSubscriber_Handle_UnknownEvent(t *testing.T) {
	push := &mockPush{}
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	sub := subscriber.New(push, log)

	msg := newMsg(t, "unknown.event", map[string]any{
		"reservation_id": "res-001",
		"driver_id":      "drv-001",
	})
	msg.Event = "unknown.event"

	err := sub.Handle(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, push.called)
	assert.Equal(t, "Notifikasi ParkirPintar", push.lastTitle)
}
