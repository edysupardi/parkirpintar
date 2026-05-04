package domain

import (
	"context"
	"time"
)

type TransactionStatus string

const (
	StatusPending  TransactionStatus = "pending"
	StatusSettled  TransactionStatus = "settled"
	StatusFailed   TransactionStatus = "failed"
	StatusExpired  TransactionStatus = "expired"
	StatusRefunded TransactionStatus = "refunded"
)

type Transaction struct {
	TransactionID  string
	InvoiceID      string
	DriverID       string
	GatewayTxID    string
	PaymentMethod  string
	Status         TransactionStatus
	Amount         int64
	PaymentURL     string
	QRString       string
	VANumber       string
	VABank         string
	IdempotencyKey string
	CreatedAt      time.Time
	PaidAt         *time.Time
	ExpiredAt      *time.Time
}

type Repository interface {
	InsertTransaction(ctx context.Context, tx Transaction) error
	GetTransaction(ctx context.Context, transactionID string) (*Transaction, error)
	GetByGatewayTxID(ctx context.Context, gatewayTxID string) (*Transaction, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*Transaction, error)
	UpdateStatus(ctx context.Context, transactionID string, status TransactionStatus, paidAt *time.Time) error
	ListByInvoice(ctx context.Context, invoiceID string) ([]Transaction, error)
}

type PaymentGateway interface {
	CreateQRIS(ctx context.Context, orderID string, amount int64, customer CustomerInfo) (qrString, paymentURL string, expiredAt time.Time, err error)
	CreateVA(ctx context.Context, orderID, bank string, amount int64, customer CustomerInfo) (vaNumber string, expiredAt time.Time, err error)
	VerifyWebhookSignature(orderID, statusCode, grossAmount, serverKey string) string
}

type CustomerInfo struct {
	Name  string
	Email string
	Phone string
}
