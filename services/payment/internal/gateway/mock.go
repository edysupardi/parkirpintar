package gateway

import (
	"context"
	"crypto/sha512"
	"fmt"
	"time"

	"github.com/edysupardi/parkirpintar/services/payment/internal/domain"
	"github.com/google/uuid"
)

type MockGateway struct {
	serverKey string
}

func NewMock(serverKey string) *MockGateway {
	return &MockGateway{serverKey: serverKey}
}

func (g *MockGateway) CreateQRIS(_ context.Context, orderID string, _ int64, _ domain.CustomerInfo) (string, string, time.Time, error) {
	qr := "mock-qr-" + uuid.New().String()[:8]
	url := "https://pay.mock/qris/" + orderID
	return qr, url, time.Now().Add(15 * time.Minute), nil
}

func (g *MockGateway) CreateVA(_ context.Context, orderID, bank string, _ int64, _ domain.CustomerInfo) (string, time.Time, error) {
	va := "8800" + orderID[:8]
	return va, time.Now().Add(24 * time.Hour), nil
}

func (g *MockGateway) VerifyWebhookSignature(orderID, statusCode, grossAmount, serverKey string) string {
	h := sha512.New()
	h.Write([]byte(orderID + statusCode + grossAmount + serverKey))
	return fmt.Sprintf("%x", h.Sum(nil))
}
