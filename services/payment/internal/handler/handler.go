package handler

import (
	"context"

	commonv1 "github.com/edysupardi/parkirpintar/gen/common/v1"
	paymentv1 "github.com/edysupardi/parkirpintar/gen/payment/v1"
	"github.com/edysupardi/parkirpintar/services/payment/internal/domain"
	"github.com/edysupardi/parkirpintar/services/payment/internal/usecase"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PaymentHandler struct {
	paymentv1.UnimplementedPaymentServiceServer
	uc *usecase.PaymentUsecase
}

func New(uc *usecase.PaymentUsecase) *PaymentHandler {
	return &PaymentHandler{uc: uc}
}

func (h *PaymentHandler) CreateTransaction(ctx context.Context, req *paymentv1.CreateTransactionRequest) (*paymentv1.CreateTransactionResponse, error) {
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}

	customer := domain.CustomerInfo{}
	if req.CustomerInfo != nil {
		customer.Name = req.CustomerInfo.Name
		customer.Email = req.CustomerInfo.Email
		customer.Phone = req.CustomerInfo.Phone
	}

	tx, err := h.uc.CreateTransaction(ctx,
		req.InvoiceId, req.DriverId, req.IdempotencyKey,
		protoToPaymentMethod(req.PaymentMethod), req.Amount.Amount, customer,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	resp := &paymentv1.CreateTransactionResponse{
		TransactionId: tx.TransactionID,
		GatewayTxId:   tx.GatewayTxID,
		Status:        paymentv1.TransactionStatus_TRANSACTION_STATUS_PENDING,
		PaymentMethod: req.PaymentMethod,
		PaymentUrl:    tx.PaymentURL,
		QrString:      tx.QRString,
		VaNumber:      tx.VANumber,
		VaBank:        tx.VABank,
	}
	if tx.ExpiredAt != nil {
		resp.ExpiredAt = timestamppb.New(*tx.ExpiredAt)
	}
	return resp, nil
}

func (h *PaymentHandler) GetTransactionStatus(ctx context.Context, req *paymentv1.GetTransactionStatusRequest) (*paymentv1.GetTransactionStatusResponse, error) {
	tx, err := h.uc.GetTransactionStatus(ctx, req.TransactionId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "%v", err)
	}

	resp := &paymentv1.GetTransactionStatusResponse{
		TransactionId: tx.TransactionID,
		InvoiceId:     tx.InvoiceID,
		Status:        domainStatusToProto(tx.Status),
		Amount:        &commonv1.Money{Amount: tx.Amount, Currency: "IDR"},
	}
	if tx.PaidAt != nil {
		resp.PaidAt = timestamppb.New(*tx.PaidAt)
	}
	return resp, nil
}

func (h *PaymentHandler) HandleWebhook(ctx context.Context, req *paymentv1.HandleWebhookRequest) (*paymentv1.HandleWebhookResponse, error) {
	processed, isDuplicate, txID, newStatus, err := h.uc.HandleWebhook(ctx, req.Payload, req.Signature)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	return &paymentv1.HandleWebhookResponse{
		Processed:     processed,
		IsDuplicate:   isDuplicate,
		TransactionId: txID,
		NewStatus:     domainStatusToProto(newStatus),
	}, nil
}

func (h *PaymentHandler) ListPaymentsByInvoice(ctx context.Context, req *paymentv1.ListPaymentsByInvoiceRequest) (*paymentv1.ListPaymentsByInvoiceResponse, error) {
	txs, err := h.uc.ListByInvoice(ctx, req.InvoiceId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	var result []*paymentv1.Transaction
	for _, tx := range txs {
		t := &paymentv1.Transaction{
			TransactionId: tx.TransactionID,
			InvoiceId:     tx.InvoiceID,
			GatewayTxId:   tx.GatewayTxID,
			Status:        domainStatusToProto(tx.Status),
			Amount:        &commonv1.Money{Amount: tx.Amount, Currency: "IDR"},
			CreatedAt:     timestamppb.New(tx.CreatedAt),
		}
		if tx.PaidAt != nil {
			t.PaidAt = timestamppb.New(*tx.PaidAt)
		}
		result = append(result, t)
	}
	return &paymentv1.ListPaymentsByInvoiceResponse{Transactions: result}, nil
}

func domainStatusToProto(s domain.TransactionStatus) paymentv1.TransactionStatus {
	switch s {
	case domain.StatusSettled:
		return paymentv1.TransactionStatus_TRANSACTION_STATUS_SETTLED
	case domain.StatusFailed:
		return paymentv1.TransactionStatus_TRANSACTION_STATUS_FAILED
	case domain.StatusExpired:
		return paymentv1.TransactionStatus_TRANSACTION_STATUS_EXPIRED
	case domain.StatusRefunded:
		return paymentv1.TransactionStatus_TRANSACTION_STATUS_REFUNDED
	default:
		return paymentv1.TransactionStatus_TRANSACTION_STATUS_PENDING
	}
}

func protoToPaymentMethod(m commonv1.PaymentMethod) string {
	switch m {
	case commonv1.PaymentMethod_PAYMENT_METHOD_QRIS:
		return "QRIS"
	case commonv1.PaymentMethod_PAYMENT_METHOD_GOPAY:
		return "GOPAY"
	case commonv1.PaymentMethod_PAYMENT_METHOD_OVO:
		return "OVO"
	case commonv1.PaymentMethod_PAYMENT_METHOD_DANA:
		return "DANA"
	case commonv1.PaymentMethod_PAYMENT_METHOD_VA_BCA:
		return "VA_BCA"
	case commonv1.PaymentMethod_PAYMENT_METHOD_VA_BNI:
		return "VA_BNI"
	case commonv1.PaymentMethod_PAYMENT_METHOD_VA_BRI:
		return "VA_BRI"
	case commonv1.PaymentMethod_PAYMENT_METHOD_VA_MANDIRI:
		return "VA_MANDIRI"
	default:
		return m.String()
	}
}
