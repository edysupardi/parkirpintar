package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/payment/internal/domain"
	"github.com/google/uuid"
)

type PaymentUsecase struct {
	repo        domain.Repository
	gateway     domain.PaymentGateway
	idempotency *idempotency.Store
	serverKey   string
	log         logger.Logger
}

func New(repo domain.Repository, gateway domain.PaymentGateway, idempotency *idempotency.Store, serverKey string, log logger.Logger) *PaymentUsecase {
	return &PaymentUsecase{repo: repo, gateway: gateway, idempotency: idempotency, serverKey: serverKey, log: log}
}

func (uc *PaymentUsecase) CreateTransaction(ctx context.Context, invoiceID, driverID, idempotencyKey, paymentMethod string, amount int64, customer domain.CustomerInfo) (*domain.Transaction, error) {
	_, hit, err := uc.idempotency.Check(ctx, "idempotency:"+idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("idempotency check: %w", err)
	}
	if hit {
		return uc.repo.GetByIdempotencyKey(ctx, idempotencyKey)
	}

	txID := uuid.New().String()
	orderID := fmt.Sprintf("PP-%s", txID[:8])

	tx := domain.Transaction{
		TransactionID:  txID,
		InvoiceID:      invoiceID,
		DriverID:       driverID,
		GatewayTxID:    orderID,
		PaymentMethod:  paymentMethod,
		Status:         domain.StatusPending,
		Amount:         amount,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      time.Now(),
	}

	switch paymentMethod {
	case "QRIS", "GOPAY", "OVO", "DANA":
		qrStr, payURL, expiredAt, err := uc.gateway.CreateQRIS(ctx, orderID, amount, customer)
		if err != nil {
			return nil, fmt.Errorf("create QRIS: %w", err)
		}
		tx.QRString = qrStr
		tx.PaymentURL = payURL
		tx.ExpiredAt = &expiredAt

	case "VA_BCA", "VA_BNI", "VA_BRI", "VA_MANDIRI":
		bank := paymentMethod[3:] // strip "VA_"
		vaNumber, expiredAt, err := uc.gateway.CreateVA(ctx, orderID, bank, amount, customer)
		if err != nil {
			return nil, fmt.Errorf("create VA: %w", err)
		}
		tx.VANumber = vaNumber
		tx.VABank = bank
		tx.ExpiredAt = &expiredAt

	default:
		return nil, fmt.Errorf("unsupported payment method: %s", paymentMethod)
	}

	if err := uc.repo.InsertTransaction(ctx, tx); err != nil {
		return nil, fmt.Errorf("insert transaction: %w", err)
	}

	if err := uc.idempotency.Save(ctx, "idempotency:"+idempotencyKey, txID, idempotency.DefaultTTL); err != nil {
		uc.log.Warn(ctx).Err(err).Msg("failed to save idempotency key")
	}

	return &tx, nil
}

func (uc *PaymentUsecase) GetTransactionStatus(ctx context.Context, transactionID string) (*domain.Transaction, error) {
	return uc.repo.GetTransaction(ctx, transactionID)
}

func (uc *PaymentUsecase) ListByInvoice(ctx context.Context, invoiceID string) ([]domain.Transaction, error) {
	return uc.repo.ListByInvoice(ctx, invoiceID)
}

type webhookPayload struct {
	OrderID     string `json:"order_id"`
	StatusCode  string `json:"status_code"`
	GrossAmount string `json:"gross_amount"`
	SignatureKey string `json:"signature_key"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus string `json:"fraud_status"`
}

func (uc *PaymentUsecase) HandleWebhook(ctx context.Context, payload []byte, signature string) (processed, isDuplicate bool, txID string, newStatus domain.TransactionStatus, err error) {
	var p webhookPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return false, false, "", "", fmt.Errorf("invalid payload: %w", err)
	}

	// verify signature
	expected := uc.gateway.VerifyWebhookSignature(p.OrderID, p.StatusCode, p.GrossAmount, uc.serverKey)
	if expected != p.SignatureKey {
		return false, false, "", "", fmt.Errorf("invalid signature")
	}

	// idempotency — use order_id + status as key
	idempotencyKey := fmt.Sprintf("webhook:%s:%s", p.OrderID, p.TransactionStatus)
	_, hit, err := uc.idempotency.Check(ctx, idempotencyKey)
	if err != nil {
		return false, false, "", "", fmt.Errorf("idempotency check: %w", err)
	}
	if hit {
		tx, _ := uc.repo.GetByGatewayTxID(ctx, p.OrderID)
		if tx != nil {
			return true, true, tx.TransactionID, tx.Status, nil
		}
		return true, true, "", "", nil
	}

	tx, err := uc.repo.GetByGatewayTxID(ctx, p.OrderID)
	if err != nil {
		return false, false, "", "", fmt.Errorf("get transaction: %w", err)
	}

	status := mapMidtransStatus(p.TransactionStatus, p.FraudStatus)
	var paidAt *time.Time
	if status == domain.StatusSettled {
		now := time.Now()
		paidAt = &now
	}

	if err := uc.repo.UpdateStatus(ctx, tx.TransactionID, status, paidAt); err != nil {
		return false, false, "", "", fmt.Errorf("update status: %w", err)
	}

	_ = uc.idempotency.Save(ctx, idempotencyKey, tx.TransactionID, idempotency.DefaultTTL)

	return true, false, tx.TransactionID, status, nil
}

func mapMidtransStatus(txStatus, fraudStatus string) domain.TransactionStatus {
	switch txStatus {
	case "capture":
		if fraudStatus == "accept" {
			return domain.StatusSettled
		}
		return domain.StatusFailed
	case "settlement":
		return domain.StatusSettled
	case "deny", "cancel", "failure":
		return domain.StatusFailed
	case "expire":
		return domain.StatusExpired
	case "refund":
		return domain.StatusRefunded
	default:
		return domain.StatusPending
	}
}
