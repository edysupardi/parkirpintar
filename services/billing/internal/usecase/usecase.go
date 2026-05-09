package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/edysupardi/parkirpintar/pkg/idempotency"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/pkg/pricing"
	"github.com/edysupardi/parkirpintar/services/billing/internal/domain"
	"github.com/google/uuid"
)

type BillingUsecase struct {
	repo        domain.Repository
	idempotency *idempotency.Store
	log         logger.Logger
}

func New(repo domain.Repository, idempotency *idempotency.Store, log logger.Logger) *BillingUsecase {
	return &BillingUsecase{repo: repo, idempotency: idempotency, log: log}
}

func (uc *BillingUsecase) GenerateInvoice(ctx context.Context, sessionID, reservationID, driverID, idempotencyKey string, checkIn, checkOut time.Time) (*domain.Invoice, error) {
	_, hit, err := uc.idempotency.Check(ctx, "idempotency:"+idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("idempotency check: %w", err)
	}
	if hit {
		return uc.repo.GetByIdempotencyKey(ctx, idempotencyKey)
	}

	result := pricing.Calculate(checkIn, checkOut)
	durationMins := int64(checkOut.Sub(checkIn).Minutes())

	inv := domain.Invoice{
		InvoiceID:      uuid.New().String(),
		Type:           domain.InvoiceTypeParkingSession,
		SessionID:      sessionID,
		ReservationID:  reservationID,
		DriverID:       driverID,
		BookingFee:     result.BookingFee,
		ParkingFee:     result.ParkingFee,
		OvernightFee:   result.OvernightFee,
		TotalAmount:    result.TotalAmount,
		BilledHours:    result.BilledHours,
		IsOvernight:    result.IsOvernight,
		DurationMins:   durationMins,
		Status:         domain.InvoiceStatusPendingPayment,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      time.Now(),
	}

	if err := uc.repo.InsertInvoice(ctx, inv); err != nil {
		return nil, fmt.Errorf("insert invoice: %w", err)
	}

	if err := uc.idempotency.Save(ctx, "idempotency:"+idempotencyKey, inv.InvoiceID, idempotency.DefaultTTL); err != nil {
		uc.log.Warn(ctx).Err(err).Msg("failed to save idempotency key")
	}

	return &inv, nil
}

func (uc *BillingUsecase) GenerateBookingFeeInvoice(ctx context.Context, reservationID, driverID, idempotencyKey string) (*domain.Invoice, error) {
	_, hit, err := uc.idempotency.Check(ctx, "idempotency:"+idempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("idempotency check: %w", err)
	}
	if hit {
		return uc.repo.GetByIdempotencyKey(ctx, idempotencyKey)
	}

	inv := domain.Invoice{
		InvoiceID:      uuid.New().String(),
		Type:           domain.InvoiceTypeBookingFee,
		ReservationID:  reservationID,
		DriverID:       driverID,
		BookingFee:     pricing.BookingFee,
		TotalAmount:    pricing.BookingFee,
		Status:         domain.InvoiceStatusPendingPayment,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      time.Now(),
	}

	if err := uc.repo.InsertInvoice(ctx, inv); err != nil {
		return nil, fmt.Errorf("insert booking fee invoice: %w", err)
	}

	if err := uc.idempotency.Save(ctx, "idempotency:"+idempotencyKey, inv.InvoiceID, idempotency.DefaultTTL); err != nil {
		uc.log.Warn(ctx).Err(err).Msg("failed to save idempotency key")
	}

	return &inv, nil
}

func (uc *BillingUsecase) GetInvoice(ctx context.Context, invoiceID string) (*domain.Invoice, error) {
	return uc.repo.GetInvoice(ctx, invoiceID)
}

func (uc *BillingUsecase) GetInvoiceBySession(ctx context.Context, sessionID string) (*domain.Invoice, error) {
	return uc.repo.GetInvoiceBySession(ctx, sessionID)
}

func (uc *BillingUsecase) PreviewBilling(ctx context.Context, checkIn time.Time) pricing.Result {
	return pricing.Calculate(checkIn, time.Now())
}

func (uc *BillingUsecase) CalculateCancellationFee(ctx context.Context, confirmedAt, cancelledAt time.Time, isNoShow bool) (int64, string) {
	fee := pricing.CalculateCancellationFee(confirmedAt, cancelledAt, isNoShow)
	var reason string
	switch fee {
	case 0:
		if isNoShow {
			reason = "No-show — booking fee already charged at confirmation, no extra charge"
		} else {
			reason = "Cancelled within 2 minutes of confirmation — no fee"
		}
	case 5_000:
		reason = "Cancelled after 2 minutes before check-in — 5.000 IDR"
	}
	return fee, reason
}

func (uc *BillingUsecase) MarkInvoicePaid(ctx context.Context, invoiceID, gatewayTxID, paymentMethod string, paidAt time.Time) (*domain.Invoice, error) {
	if err := uc.repo.MarkPaid(ctx, invoiceID, gatewayTxID, paymentMethod, paidAt); err != nil {
		return nil, fmt.Errorf("mark paid: %w", err)
	}
	return uc.repo.GetInvoice(ctx, invoiceID)
}
