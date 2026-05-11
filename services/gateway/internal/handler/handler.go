package handler

import (
	"context"
	"io"

	billingv1 "github.com/edysupardi/parkirpintar/gen/billing/v1"
	commonv1 "github.com/edysupardi/parkirpintar/gen/common/v1"
	gatewayv1 "github.com/edysupardi/parkirpintar/gen/gateway/v1"
	paymentv1 "github.com/edysupardi/parkirpintar/gen/payment/v1"
	presencev1 "github.com/edysupardi/parkirpintar/gen/presence/v1"
	reservationv1 "github.com/edysupardi/parkirpintar/gen/reservation/v1"
	"github.com/edysupardi/parkirpintar/pkg/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type GatewayHandler struct {
	gatewayv1.UnimplementedGatewayServiceServer
	reservation reservationv1.ReservationServiceClient
	billing     billingv1.BillingServiceClient
	payment     paymentv1.PaymentServiceClient
	presence    presencev1.PresenceServiceClient
	extra       *ExtraHandler
}

func New(
	reservation reservationv1.ReservationServiceClient,
	billing billingv1.BillingServiceClient,
	payment paymentv1.PaymentServiceClient,
	presence presencev1.PresenceServiceClient,
	extra *ExtraHandler,
) *GatewayHandler {
	return &GatewayHandler{
		reservation: reservation,
		billing:     billing,
		payment:     payment,
		presence:    presence,
		extra:       extra,
	}
}

func (h *GatewayHandler) GetParkingAvailability(ctx context.Context, req *gatewayv1.GetParkingAvailabilityRequest) (*gatewayv1.GetParkingAvailabilityResponse, error) {
	resp, err := h.reservation.GetAvailability(ctx, &reservationv1.GetAvailabilityRequest{
		VehicleType: req.VehicleType,
	})
	if err != nil {
		return nil, err
	}
	return &gatewayv1.GetParkingAvailabilityResponse{
		VehicleType:    resp.VehicleType,
		TotalCapacity:  resp.TotalCapacity,
		AvailableSpots: resp.AvailableSpots,
		IsAvailable:    resp.IsAvailable,
	}, nil
}

func (h *GatewayHandler) CreateReservation(ctx context.Context, req *gatewayv1.CreateReservationRequest) (*gatewayv1.CreateReservationResponse, error) {
	driverID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing user id")
	}

	resp, err := h.reservation.CreateReservation(ctx, &reservationv1.CreateReservationRequest{
		DriverId:       driverID,
		VehicleType:    req.VehicleType,
		AssignmentMode: req.AssignmentMode,
		SpotId:         req.SpotId,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	// Create booking fee invoice + QRIS payment
	invoiceID, err := h.extra.CreateBookingFeeInvoice(ctx, resp.ReservationId, driverID, "booking-"+req.IdempotencyKey)
	if err != nil {
		_, _ = h.reservation.CancelReservation(ctx, &reservationv1.CancelReservationRequest{
			ReservationId: resp.ReservationId,
			DriverId:      driverID,
		})
		return nil, status.Errorf(codes.FailedPrecondition, "booking fee invoice failed: %v", err)
	}

	// Create QRIS payment via payment service
	payResp, err := h.payment.CreateTransaction(ctx, &paymentv1.CreateTransactionRequest{
		InvoiceId:      invoiceID,
		DriverId:       driverID,
		Amount:         &commonv1.Money{Amount: 5000, Currency: "IDR"},
		PaymentMethod:  commonv1.PaymentMethod_PAYMENT_METHOD_QRIS,
		IdempotencyKey: "pay-booking-" + req.IdempotencyKey,
	})
	if err != nil {
		_, _ = h.reservation.CancelReservation(ctx, &reservationv1.CancelReservationRequest{
			ReservationId: resp.ReservationId,
			DriverId:      driverID,
		})
		return nil, status.Errorf(codes.FailedPrecondition, "booking fee payment failed: %v", err)
	}

	return &gatewayv1.CreateReservationResponse{
		ReservationId: resp.ReservationId,
		Spot:          resp.Spot,
		Status:        resp.Status,
		ConfirmedAt:   resp.ConfirmedAt,
		ExpiresAt:     resp.ExpiresAt,
		BookingFee:    &commonv1.Money{Amount: 5000, Currency: "IDR"},
		Navigation: &gatewayv1.SpotNavigation{
			Floor:      resp.Spot.Floor,
			SpotNumber: resp.Spot.Number,
			Direction:  payResp.QrString + "|" + payResp.TransactionId,
		},
	}, nil
}

func (h *GatewayHandler) GetMyReservation(ctx context.Context, _ *gatewayv1.GetMyReservationRequest) (*gatewayv1.GetMyReservationResponse, error) {
	driverID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing user id")
	}

	resp, err := h.reservation.GetActiveReservation(ctx, &reservationv1.GetActiveReservationRequest{
		DriverId: driverID,
	})
	if err != nil {
		return nil, err
	}
	if !resp.HasActive {
		return &gatewayv1.GetMyReservationResponse{HasActiveReservation: false}, nil
	}

	r := resp.Reservation
	res := &gatewayv1.Reservation{
		ReservationId: r.ReservationId,
		Spot:          r.Spot,
		Status:        r.Status,
		ConfirmedAt:   r.ConfirmedAt,
		ExpiresAt:     r.ExpiresAt,
		SessionId:     r.SessionId,
	}
	if r.CancelledAt != nil {
		res.CheckInAt = r.CancelledAt
	}
	return &gatewayv1.GetMyReservationResponse{
		HasActiveReservation: true,
		Reservation:          res,
	}, nil
}

func (h *GatewayHandler) CancelReservation(ctx context.Context, req *gatewayv1.CancelReservationRequest) (*gatewayv1.CancelReservationResponse, error) {
	driverID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing user id")
	}

	resp, err := h.reservation.CancelReservation(ctx, &reservationv1.CancelReservationRequest{
		ReservationId: req.ReservationId,
		DriverId:      driverID,
	})
	if err != nil {
		return nil, err
	}
	return &gatewayv1.CancelReservationResponse{
		ReservationId:   resp.ReservationId,
		Status:          resp.Status,
		CancellationFee: resp.CancellationFee,
		FeeReason:       resp.FeeReason,
	}, nil
}

func (h *GatewayHandler) CheckIn(ctx context.Context, req *gatewayv1.CheckInRequest) (*gatewayv1.CheckInResponse, error) {
	driverID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing user id")
	}

	resp, err := h.reservation.CheckIn(ctx, &reservationv1.CheckInRequest{
		ReservationId: req.ReservationId,
		DriverId:      driverID,
		Location:      req.Coordinate,
	})
	if err != nil {
		return nil, err
	}
	return &gatewayv1.CheckInResponse{
		ReservationId: resp.ReservationId,
		SessionId:     resp.SessionId,
		Status:        resp.Status,
		CheckInAt:     resp.CheckInAt,
	}, nil
}

func (h *GatewayHandler) CheckOut(ctx context.Context, req *gatewayv1.CheckOutRequest) (*gatewayv1.CheckOutResponse, error) {
	driverID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing user id")
	}

	resResp, err := h.reservation.CheckOut(ctx, &reservationv1.CheckOutRequest{
		ReservationId:  req.ReservationId,
		DriverId:       driverID,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	// fetch full reservation to get check_in_at
	resDetail, err := h.reservation.GetReservation(ctx, &reservationv1.GetReservationRequest{
		ReservationId: req.ReservationId,
	})
	if err != nil {
		return nil, err
	}

	// generate invoice via billing service
	invResp, err := h.billing.GenerateInvoice(ctx, &billingv1.GenerateInvoiceRequest{
		SessionId:      resResp.SessionId,
		ReservationId:  req.ReservationId,
		DriverId:       driverID,
		IdempotencyKey: "inv-" + req.IdempotencyKey,
		CheckInAt:      resDetail.Reservation.ConfirmedAt,
		CheckOutAt:     resResp.CheckOutAt,
	})
	if err != nil {
		return nil, err
	}

	inv := invResp.Invoice
	return &gatewayv1.CheckOutResponse{
		SessionId:       resResp.SessionId,
		InvoiceId:       inv.InvoiceId,
		CheckOutAt:      resResp.CheckOutAt,
		DurationMinutes: resResp.DurationMinutes,
		InvoiceSummary: &gatewayv1.InvoiceSummary{
			InvoiceId:    inv.InvoiceId,
			BookingFee:   inv.Breakdown.BookingFee,
			ParkingFee:   inv.Breakdown.ParkingFee,
			OvernightFee: inv.Breakdown.OvernightFee,
			TotalAmount:  inv.TotalAmount,
			Status:       inv.Status.String(),
			BilledHours:  inv.Breakdown.BilledHours,
		},
	}, nil
}

func (h *GatewayHandler) GetInvoice(ctx context.Context, req *gatewayv1.GetInvoiceRequest) (*gatewayv1.GetInvoiceResponse, error) {
	resp, err := h.billing.GetInvoice(ctx, &billingv1.GetInvoiceRequest{InvoiceId: req.InvoiceId})
	if err != nil {
		return nil, err
	}
	inv := resp.Invoice
	return &gatewayv1.GetInvoiceResponse{
		Invoice: &gatewayv1.InvoiceSummary{
			InvoiceId:    inv.InvoiceId,
			BookingFee:   inv.Breakdown.BookingFee,
			ParkingFee:   inv.Breakdown.ParkingFee,
			OvernightFee: inv.Breakdown.OvernightFee,
			TotalAmount:  inv.TotalAmount,
			Status:       inv.Status.String(),
			BilledHours:  inv.Breakdown.BilledHours,
		},
	}, nil
}

func (h *GatewayHandler) PreviewBilling(ctx context.Context, req *gatewayv1.PreviewBillingRequest) (*gatewayv1.PreviewBillingResponse, error) {
	resp, err := h.billing.GetInvoiceBySession(ctx, &billingv1.GetInvoiceBySessionRequest{SessionId: req.SessionId})
	if err != nil {
		return nil, err
	}
	inv := resp.Invoice
	return &gatewayv1.PreviewBillingResponse{
		Estimated: &gatewayv1.InvoiceSummary{
			InvoiceId:    inv.InvoiceId,
			BookingFee:   inv.Breakdown.BookingFee,
			ParkingFee:   inv.Breakdown.ParkingFee,
			OvernightFee: inv.Breakdown.OvernightFee,
			TotalAmount:  inv.TotalAmount,
			BilledHours:  inv.Breakdown.BilledHours,
		},
		Note: "Estimasi, final amount dihitung saat checkout",
	}, nil
}

func (h *GatewayHandler) CreatePayment(ctx context.Context, req *gatewayv1.CreatePaymentRequest) (*gatewayv1.CreatePaymentResponse, error) {
	driverID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing user id")
	}

	inv, err := h.billing.GetInvoice(ctx, &billingv1.GetInvoiceRequest{InvoiceId: req.InvoiceId})
	if err != nil {
		return nil, err
	}

	resp, err := h.payment.CreateTransaction(ctx, &paymentv1.CreateTransactionRequest{
		InvoiceId:      req.InvoiceId,
		DriverId:       driverID,
		Amount:         inv.Invoice.TotalAmount,
		PaymentMethod:  req.PaymentMethod,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}

	gwResp := &gatewayv1.CreatePaymentResponse{
		TransactionId: resp.TransactionId,
		PaymentMethod: resp.PaymentMethod,
		PaymentUrl:    resp.PaymentUrl,
		QrString:      resp.QrString,
		VaNumber:      resp.VaNumber,
		VaBank:        resp.VaBank,
	}
	if resp.ExpiredAt != nil {
		gwResp.ExpiredAt = resp.ExpiredAt
	}
	return gwResp, nil
}

func (h *GatewayHandler) GetPaymentStatus(ctx context.Context, req *gatewayv1.GetPaymentStatusRequest) (*gatewayv1.GetPaymentStatusResponse, error) {
	resp, err := h.payment.GetTransactionStatus(ctx, &paymentv1.GetTransactionStatusRequest{
		TransactionId: req.TransactionId,
	})
	if err != nil {
		return nil, err
	}
	gwResp := &gatewayv1.GetPaymentStatusResponse{
		TransactionId: resp.TransactionId,
		Status:        resp.Status.String(),
		Amount:        resp.Amount,
	}
	if resp.PaidAt != nil {
		gwResp.PaidAt = resp.PaidAt
	}
	return gwResp, nil
}

func (h *GatewayHandler) StreamMyLocation(stream gatewayv1.GatewayService_StreamMyLocationServer) error {
	ctx := stream.Context()
	driverID, ok := auth.UserIDFromContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing user id")
	}

	presenceStream, err := h.presence.StreamLocation(ctx)
	if err != nil {
		return err
	}

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return presenceStream.CloseSend()
		}
		if err != nil {
			return err
		}

		if err := presenceStream.Send(&presencev1.StreamLocationRequest{
			DriverId:      driverID,
			SessionId:     req.SessionId,
			ReservationId: req.ReservationId,
			Coordinate:    req.Coordinate,
			RecordedAt:    req.RecordedAt,
		}); err != nil {
			return err
		}

		presResp, err := presenceStream.Recv()
		if err != nil {
			return err
		}

		ack := &gatewayv1.StreamMyLocationResponse{
			EventType: "ack",
		}
		if presResp.LocationAck != nil {
			ack.InGeofence = presResp.LocationAck.InGeofence
			ack.DistanceToSpot = presResp.LocationAck.DistanceToSpot
		}
		if err := stream.Send(ack); err != nil {
			return err
		}
	}
}

func buildDirection(floor, number int32) string {
	return "Lantai " + itoa(floor) + ", Spot " + itoa(number)
}

func itoa(n int32) string {
	return string(rune('0' + n%10))
}

// ensure timestamppb is used
var _ = timestamppb.Now
var _ = commonv1.VehicleType_VEHICLE_TYPE_CAR
