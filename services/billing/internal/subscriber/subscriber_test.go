package subscriber_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/pkg/mq"
	"github.com/edysupardi/parkirpintar/services/billing/internal/domain"
	"github.com/edysupardi/parkirpintar/services/billing/internal/subscriber"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockInvoiceGenerator struct {
	called bool
	err    error
}

func (m *mockInvoiceGenerator) GenerateInvoice(_ context.Context, _, _, _, _ string, _, _ time.Time) (*domain.Invoice, error) {
	m.called = true
	if m.err != nil {
		return nil, m.err
	}
	return &domain.Invoice{InvoiceID: "inv-001"}, nil
}

func newMsg(t *testing.T, payload any) mq.Message {
	t.Helper()
	raw, err := json.Marshal(payload)
	require.NoError(t, err)
	return mq.Message{
		Event:     mq.EventCheckOutCompleted,
		Timestamp: time.Now(),
		Payload:   raw,
	}
}

func TestBillingSubscriber_HandleCheckOut_HappyPath(t *testing.T) {
	gen := &mockInvoiceGenerator{}
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	sub := subscriber.New(gen, log)

	msg := newMsg(t, map[string]string{
		"reservation_id": "res-001",
		"driver_id":      "drv-001",
		"session_id":     "sess-001",
	})

	err := sub.HandleCheckOut(context.Background(), msg)
	require.NoError(t, err)
	assert.True(t, gen.called)
}

func TestBillingSubscriber_HandleCheckOut_MissingSessionID(t *testing.T) {
	gen := &mockInvoiceGenerator{}
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	sub := subscriber.New(gen, log)

	msg := newMsg(t, map[string]string{
		"reservation_id": "res-001",
		"driver_id":      "drv-001",
		"session_id":     "",
	})

	err := sub.HandleCheckOut(context.Background(), msg)
	require.NoError(t, err)
	assert.False(t, gen.called, "should skip when session_id is empty")
}

func TestBillingSubscriber_HandleCheckOut_GenerateInvoiceError(t *testing.T) {
	gen := &mockInvoiceGenerator{err: errors.New("db error")}
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	sub := subscriber.New(gen, log)

	msg := newMsg(t, map[string]string{
		"reservation_id": "res-001",
		"driver_id":      "drv-001",
		"session_id":     "sess-001",
	})

	err := sub.HandleCheckOut(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db error")
}

func TestBillingSubscriber_HandleCheckOut_InvalidPayload(t *testing.T) {
	gen := &mockInvoiceGenerator{}
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	sub := subscriber.New(gen, log)

	msg := mq.Message{
		Event:     mq.EventCheckOutCompleted,
		Timestamp: time.Now(),
		Payload:   []byte("not-json"),
	}

	err := sub.HandleCheckOut(context.Background(), msg)
	require.Error(t, err)
	assert.False(t, gen.called)
}
