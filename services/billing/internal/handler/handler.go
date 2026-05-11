package handler

import (
	"context"
	"time"

	billingv1 "github.com/edysupardi/parkirpintar/gen/billing/v1"
	commonv1 "github.com/edysupardi/parkirpintar/gen/common/v1"
	"github.com/edysupardi/parkirpintar/services/billing/internal/domain"
	"github.com/edysupardi/parkirpintar/services/billing/internal/usecase"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type BillingHandler struct {
	billingv1.UnimplementedBillingServiceServer
	uc *usecase.BillingUsecase
}

func New(uc *usecase.BillingUsecase) *BillingHandler {
	return &BillingHandler{uc: uc}
}

func (h *BillingHandler) GenerateInvoice(ctx context.Context, req *billingv1.GenerateInvoiceRequest) (*billingv1.GenerateInvoiceResponse, error) {
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}

	inv, err := h.uc.GenerateInvoice(ctx,
		req.SessionId, req.ReservationId, req.DriverId, req.IdempotencyKey,
		req.CheckInAt.AsTime(), req.CheckOutAt.AsTime(),
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate invoice: %v", err)
	}

	return &billingv1.GenerateInvoiceResponse{
		InvoiceId:   inv.InvoiceID,
		Invoice:     domainInvoiceToProto(inv),
		GeneratedAt: timestamppb.New(inv.CreatedAt),
	}, nil
}

func (h *BillingHandler) GetInvoice(ctx context.Context, req *billingv1.GetInvoiceRequest) (*billingv1.GetInvoiceResponse, error) {
	inv, err := h.uc.GetInvoice(ctx, req.InvoiceId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &billingv1.GetInvoiceResponse{Invoice: domainInvoiceToProto(inv)}, nil
}

func (h *BillingHandler) GetInvoiceBySession(ctx context.Context, req *billingv1.GetInvoiceBySessionRequest) (*billingv1.GetInvoiceBySessionResponse, error) {
	inv, err := h.uc.GetInvoiceBySession(ctx, req.SessionId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}
	return &billingv1.GetInvoiceBySessionResponse{Invoice: domainInvoiceToProto(inv)}, nil
}

func (h *BillingHandler) PreviewBilling(ctx context.Context, req *billingv1.PreviewBillingRequest) (*billingv1.PreviewBillingResponse, error) {
	result := h.uc.PreviewBilling(ctx, req.CheckInAt.AsTime())
	return &billingv1.PreviewBillingResponse{
		Breakdown: &billingv1.BillingBreakdown{
			BookingFee:            money(result.BookingFee),
			ParkingFee:            money(result.ParkingFee),
			OvernightFee:          money(result.OvernightFee),
			BilledHours:           result.BilledHours,
			ActualDurationMinutes: int64(time.Since(req.CheckInAt.AsTime()).Minutes()),
			IsOvernight:           result.IsOvernight,
		},
		EstimatedTotal: money(result.TotalAmount),
		Note:           "Estimasi, final amount dihitung saat checkout",
	}, nil
}

func (h *BillingHandler) CalculateCancellationFee(ctx context.Context, req *billingv1.CalculateCancellationFeeRequest) (*billingv1.CalculateCancellationFeeResponse, error) {
	fee, reason := h.uc.CalculateCancellationFee(ctx,
		req.ConfirmedAt.AsTime(), req.CancelledAt.AsTime(), req.IsNoShow)
	return &billingv1.CalculateCancellationFeeResponse{
		Fee:    money(fee),
		Reason: reason,
	}, nil
}

func (h *BillingHandler) MarkInvoicePaid(ctx context.Context, req *billingv1.MarkInvoicePaidRequest) (*billingv1.MarkInvoicePaidResponse, error) {
	inv, err := h.uc.MarkInvoicePaid(ctx,
		req.InvoiceId, req.GatewayTxId, req.PaymentMethod.String(), req.PaidAt.AsTime())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	return &billingv1.MarkInvoicePaidResponse{
		InvoiceId: inv.InvoiceID,
		Status:    commonv1.InvoiceStatus_INVOICE_STATUS_PAID,
	}, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func money(amount int64) *commonv1.Money {
	return &commonv1.Money{Amount: amount, Currency: "IDR"}
}

func domainInvoiceToProto(inv *domain.Invoice) *billingv1.Invoice {
	i := &billingv1.Invoice{
		InvoiceId:     inv.InvoiceID,
		SessionId:     inv.SessionID,
		ReservationId: inv.ReservationID,
		DriverId:      inv.DriverID,
		Breakdown: &billingv1.BillingBreakdown{
			BookingFee:            money(inv.BookingFee),
			ParkingFee:            money(inv.ParkingFee),
			OvernightFee:          money(inv.OvernightFee),
			BilledHours:           inv.BilledHours,
			ActualDurationMinutes: inv.DurationMins,
			IsOvernight:           inv.IsOvernight,
		},
		TotalAmount: money(inv.TotalAmount),
		Status:      domainStatusToProto(inv.Status),
		CreatedAt:   timestamppb.New(inv.CreatedAt),
	}
	if inv.PaidAt != nil {
		i.PaidAt = timestamppb.New(*inv.PaidAt)
	}
	return i
}

func domainStatusToProto(s domain.InvoiceStatus) commonv1.InvoiceStatus {
	switch s {
	case domain.InvoiceStatusPendingPayment:
		return commonv1.InvoiceStatus_INVOICE_STATUS_PENDING_PAYMENT
	case domain.InvoiceStatusPaid:
		return commonv1.InvoiceStatus_INVOICE_STATUS_PAID
	case domain.InvoiceStatusVoid:
		return commonv1.InvoiceStatus_INVOICE_STATUS_VOID
	default:
		return commonv1.InvoiceStatus_INVOICE_STATUS_DRAFT
	}
}
