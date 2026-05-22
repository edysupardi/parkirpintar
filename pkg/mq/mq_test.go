package mq_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/mq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessage_MarshalUnmarshal(t *testing.T) {
	payload := map[string]string{"reservation_id": "res-123", "driver_id": "drv-456"}
	payloadBytes, _ := json.Marshal(payload)

	msg := mq.Message{
		Event:     mq.EventCheckOutCompleted,
		Timestamp: time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC),
		Payload:   payloadBytes,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded mq.Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, mq.EventCheckOutCompleted, decoded.Event)
	assert.Equal(t, msg.Timestamp, decoded.Timestamp)

	var decodedPayload map[string]string
	err = json.Unmarshal(decoded.Payload, &decodedPayload)
	require.NoError(t, err)
	assert.Equal(t, "res-123", decodedPayload["reservation_id"])
	assert.Equal(t, "drv-456", decodedPayload["driver_id"])
}

func TestMessage_EmptyPayload(t *testing.T) {
	msg := mq.Message{
		Event:     mq.EventReservationConfirmed,
		Timestamp: time.Now(),
		Payload:   nil,
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var decoded mq.Message
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)
	assert.Equal(t, mq.EventReservationConfirmed, decoded.Event)
	assert.Equal(t, json.RawMessage("null"), decoded.Payload)
}

func TestEventConstants(t *testing.T) {
	assert.Equal(t, "reservation.confirmed", mq.EventReservationConfirmed)
	assert.Equal(t, "reservation.expired", mq.EventReservationExpired)
	assert.Equal(t, "reservation.cancelled", mq.EventReservationCancelled)
	assert.Equal(t, "checkin.detected", mq.EventCheckInDetected)
	assert.Equal(t, "checkout.completed", mq.EventCheckOutCompleted)
	assert.Equal(t, "parkirpintar", mq.ExchangeParkirPintar)
}
