package handler

import (
	"context"
	"strings"

	commonv1 "github.com/edysupardi/parkirpintar/gen/common/v1"
	reservationv1 "github.com/edysupardi/parkirpintar/gen/reservation/v1"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/domain"
	"github.com/edysupardi/parkirpintar/services/reservation/internal/usecase"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type ReservationHandler struct {
	reservationv1.UnimplementedReservationServiceServer
	uc *usecase.ReservationUsecase
}

func New(uc *usecase.ReservationUsecase) *ReservationHandler {
	return &ReservationHandler{uc: uc}
}

func (h *ReservationHandler) CreateReservation(ctx context.Context, req *reservationv1.CreateReservationRequest) (*reservationv1.CreateReservationResponse, error) {
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}

	vehicleType := protoToVehicleType(req.VehicleType)
	mode := protoToAssignmentMode(req.AssignmentMode)

	r, err := h.uc.CreateReservation(ctx, req.DriverId, req.IdempotencyKey, vehicleType, mode, req.SpotId)
	if err != nil {
		return nil, toGRPCError(err)
	}

	return &reservationv1.CreateReservationResponse{
		ReservationId: r.ReservationID,
		Spot:          domainSpotToProto(r.Spot),
		Status:        domainStatusToProto(r.Status),
		ConfirmedAt:   timestamppb.New(r.ConfirmedAt),
		ExpiresAt:     timestamppb.New(r.ExpiresAt),
		BookingFee:    &commonv1.Money{Amount: 5_000, Currency: "IDR"},
	}, nil
}

func (h *ReservationHandler) CancelReservation(ctx context.Context, req *reservationv1.CancelReservationRequest) (*reservationv1.CancelReservationResponse, error) {
	r, fee, err := h.uc.CancelReservation(ctx, req.ReservationId, req.DriverId)
	if err != nil {
		return nil, toGRPCError(err)
	}

	resp := &reservationv1.CancelReservationResponse{
		ReservationId:   r.ReservationID,
		Status:          domainStatusToProto(r.Status),
		CancellationFee: &commonv1.Money{Amount: fee, Currency: "IDR"},
		CancelledAt:     timestamppb.New(*r.CancelledAt),
	}
	if fee == 0 {
		resp.FeeReason = "cancelled within 2 minutes of confirmation"
	} else {
		resp.FeeReason = "cancelled after 2 minutes of confirmation"
	}
	return resp, nil
}

func (h *ReservationHandler) GetReservation(ctx context.Context, req *reservationv1.GetReservationRequest) (*reservationv1.GetReservationResponse, error) {
	r, err := h.uc.GetReservation(ctx, req.ReservationId)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &reservationv1.GetReservationResponse{Reservation: domainReservationToProto(r)}, nil
}

func (h *ReservationHandler) GetActiveReservation(ctx context.Context, req *reservationv1.GetActiveReservationRequest) (*reservationv1.GetActiveReservationResponse, error) {
	r, err := h.uc.GetActiveReservation(ctx, req.DriverId)
	if err != nil {
		return nil, toGRPCError(err)
	}
	if r == nil {
		return &reservationv1.GetActiveReservationResponse{HasActive: false}, nil
	}
	return &reservationv1.GetActiveReservationResponse{
		Reservation: domainReservationToProto(r),
		HasActive:   true,
	}, nil
}

func (h *ReservationHandler) CheckIn(ctx context.Context, req *reservationv1.CheckInRequest) (*reservationv1.CheckInResponse, error) {
	r, err := h.uc.CheckIn(ctx, req.ReservationId, req.DriverId)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &reservationv1.CheckInResponse{
		ReservationId: r.ReservationID,
		SessionId:     r.SessionID,
		Status:        domainStatusToProto(r.Status),
		CheckInAt:     timestamppb.New(*r.CheckInAt),
	}, nil
}

func (h *ReservationHandler) CheckOut(ctx context.Context, req *reservationv1.CheckOutRequest) (*reservationv1.CheckOutResponse, error) {
	if req.IdempotencyKey == "" {
		return nil, status.Error(codes.InvalidArgument, "idempotency_key is required")
	}
	r, err := h.uc.CheckOut(ctx, req.ReservationId, req.DriverId, req.IdempotencyKey)
	if err != nil {
		return nil, toGRPCError(err)
	}
	duration := int64(r.CheckOutAt.Sub(*r.CheckInAt).Minutes())
	return &reservationv1.CheckOutResponse{
		ReservationId:   r.ReservationID,
		SessionId:       r.SessionID,
		Status:          domainStatusToProto(r.Status),
		CheckOutAt:      timestamppb.New(*r.CheckOutAt),
		DurationMinutes: duration,
	}, nil
}

func (h *ReservationHandler) GetAvailability(ctx context.Context, req *reservationv1.GetAvailabilityRequest) (*reservationv1.GetAvailabilityResponse, error) {
	vt := protoToVehicleType(req.VehicleType)
	total, available, occupied, reserved, err := h.uc.GetAvailability(ctx, vt)
	if err != nil {
		return nil, toGRPCError(err)
	}
	return &reservationv1.GetAvailabilityResponse{
		VehicleType:    req.VehicleType,
		TotalCapacity:  total,
		AvailableSpots: available,
		OccupiedSpots:  occupied,
		ReservedSpots:  reserved,
		IsAvailable:    available > 0,
	}, nil
}

func (h *ReservationHandler) ListAvailableSpots(ctx context.Context, req *reservationv1.ListAvailableSpotsRequest) (*reservationv1.ListAvailableSpotsResponse, error) {
	vt := protoToVehicleType(req.VehicleType)
	spots, err := h.uc.ListAvailableSpots(ctx, vt, req.Floor)
	if err != nil {
		return nil, toGRPCError(err)
	}
	var result []*reservationv1.SpotAvailability
	for _, s := range spots {
		result = append(result, &reservationv1.SpotAvailability{
			Spot:      domainSpotToProto(s),
			Available: true,
		})
	}
	return &reservationv1.ListAvailableSpotsResponse{Spots: result}, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func protoToVehicleType(v commonv1.VehicleType) domain.VehicleType {
	if v == commonv1.VehicleType_VEHICLE_TYPE_MOTORCYCLE {
		return domain.VehicleTypeMotorcycle
	}
	return domain.VehicleTypeCar
}

func protoToAssignmentMode(m commonv1.AssignmentMode) domain.AssignmentMode {
	if m == commonv1.AssignmentMode_ASSIGNMENT_MODE_USER_SELECTED {
		return domain.AssignmentModeSelected
	}
	return domain.AssignmentModeSystem
}

func domainStatusToProto(s domain.ReservationStatus) commonv1.ReservationStatus {
	switch s {
	case domain.StatusConfirmed:
		return commonv1.ReservationStatus_RESERVATION_STATUS_CONFIRMED
	case domain.StatusActive:
		return commonv1.ReservationStatus_RESERVATION_STATUS_ACTIVE
	case domain.StatusCompleted:
		return commonv1.ReservationStatus_RESERVATION_STATUS_COMPLETED
	case domain.StatusCancelled:
		return commonv1.ReservationStatus_RESERVATION_STATUS_CANCELLED
	case domain.StatusExpired:
		return commonv1.ReservationStatus_RESERVATION_STATUS_EXPIRED
	default:
		return commonv1.ReservationStatus_RESERVATION_STATUS_UNSPECIFIED
	}
}

func domainSpotToProto(s domain.Spot) *commonv1.Spot {
	vt := commonv1.VehicleType_VEHICLE_TYPE_CAR
	if s.VehicleType == domain.VehicleTypeMotorcycle {
		vt = commonv1.VehicleType_VEHICLE_TYPE_MOTORCYCLE
	}
	return &commonv1.Spot{
		SpotId:      s.SpotID,
		Floor:       s.Floor,
		Number:      s.SpotNumber,
		VehicleType: vt,
	}
}

func domainReservationToProto(r *domain.Reservation) *reservationv1.Reservation {
	res := &reservationv1.Reservation{
		ReservationId:  r.ReservationID,
		DriverId:       r.DriverID,
		Spot:           domainSpotToProto(r.Spot),
		Status:         domainStatusToProto(r.Status),
		ConfirmedAt:    timestamppb.New(r.ConfirmedAt),
		ExpiresAt:      timestamppb.New(r.ExpiresAt),
		SessionId:      r.SessionID,
	}
	if r.CancelledAt != nil {
		res.CancelledAt = timestamppb.New(*r.CancelledAt)
	}
	return res
}

func toGRPCError(err error) error {
	msg := err.Error()

	if strings.Contains(msg, "no rows in result set") || strings.Contains(msg, "not found") {
		return status.Error(codes.NotFound, msg)
	}
	if strings.Contains(msg, "cannot be cancelled") || strings.Contains(msg, "cannot confirm") ||
		strings.Contains(msg, "cannot check in") || strings.Contains(msg, "cannot check out") {
		return status.Error(codes.FailedPrecondition, msg)
	}

	switch msg {
	case "spot unavailable", "no available spot":
		return status.Error(codes.ResourceExhausted, msg)
	case "reservation not owned by driver":
		return status.Error(codes.PermissionDenied, msg)
	case "reservation expired":
		return status.Error(codes.FailedPrecondition, msg)
	default:
		return status.Error(codes.Internal, msg)
	}
}
