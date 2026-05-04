package domain

import (
	"context"
	"time"
)

type InvoiceStatus string

const (
	InvoiceStatusDraft          InvoiceStatus = "draft"
	InvoiceStatusPendingPayment InvoiceStatus = "pending_payment"
	InvoiceStatusPaid           InvoiceStatus = "paid"
	InvoiceStatusVoid           InvoiceStatus = "void"
)

type Invoice struct {
	InvoiceID      string
	SessionID      string
	ReservationID  string
	DriverID       string
	BookingFee     int64
	ParkingFee     int64
	OvernightFee   int64
	TotalAmount    int64
	BilledHours    int32
	IsOvernight    bool
	DurationMins   int64
	Status         InvoiceStatus
	GatewayTxID    string
	PaymentMethod  string
	CreatedAt      time.Time
	PaidAt         *time.Time
	IdempotencyKey string
}

type Repository interface {
	InsertInvoice(ctx context.Context, inv Invoice) error
	GetInvoice(ctx context.Context, invoiceID string) (*Invoice, error)
	GetInvoiceBySession(ctx context.Context, sessionID string) (*Invoice, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*Invoice, error)
	MarkPaid(ctx context.Context, invoiceID, gatewayTxID, paymentMethod string, paidAt time.Time) error
}
