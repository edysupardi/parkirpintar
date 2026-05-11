package handler

import (
	"context"
	"io"
	"time"

	commonv1 "github.com/edysupardi/parkirpintar/gen/common/v1"
	presencev1 "github.com/edysupardi/parkirpintar/gen/presence/v1"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/presence/internal/domain"
	"github.com/edysupardi/parkirpintar/services/presence/internal/repository"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type PresenceHandler struct {
	presencev1.UnimplementedPresenceServiceServer
	repo *repository.PostgresRepository
	log  logger.Logger
}

func New(repo *repository.PostgresRepository, log logger.Logger) *PresenceHandler {
	return &PresenceHandler{repo: repo, log: log}
}

func (h *PresenceHandler) StreamLocation(stream presencev1.PresenceService_StreamLocationServer) error {
	ctx := stream.Context()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "recv error: %v", err)
		}

		update := domain.LocationUpdate{
			DriverID:      req.DriverId,
			SessionID:     req.SessionId,
			ReservationID: req.ReservationId,
			RecordedAt:    time.Now(),
		}
		if req.Coordinate != nil {
			update.Latitude = req.Coordinate.Latitude
			update.Longitude = req.Coordinate.Longitude
			update.Accuracy = req.Coordinate.AccuracyMeters
		}
		if req.RecordedAt != nil {
			update.RecordedAt = req.RecordedAt.AsTime()
		}

		if err := h.repo.SaveLocation(ctx, update); err != nil {
			h.log.Warn(ctx).Err(err).Msg("failed to save location")
		}

		if err := stream.Send(&presencev1.StreamLocationResponse{
			Type:        presencev1.PresenceEventType_PRESENCE_EVENT_TYPE_ACK,
			DriverId:    req.DriverId,
			EventAt:     timestamppb.Now(),
			LocationAck: &presencev1.LocationAck{InGeofence: false},
		}); err != nil {
			return status.Errorf(codes.Internal, "send error: %v", err)
		}
	}
}

func (h *PresenceHandler) GetLastLocation(ctx context.Context, req *presencev1.GetLastLocationRequest) (*presencev1.GetLastLocationResponse, error) {
	u, err := h.repo.GetLastLocation(ctx, req.DriverId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "%v", err)
	}
	if u == nil {
		return &presencev1.GetLastLocationResponse{HasLocation: false}, nil
	}
	return &presencev1.GetLastLocationResponse{
		DriverId: u.DriverID,
		Coordinate: &commonv1.Coordinate{
			Latitude:       u.Latitude,
			Longitude:      u.Longitude,
			AccuracyMeters: u.Accuracy,
		},
		RecordedAt:  timestamppb.New(u.RecordedAt),
		HasLocation: true,
	}, nil
}

func (h *PresenceHandler) CheckGeofence(ctx context.Context, req *presencev1.CheckGeofenceRequest) (*presencev1.CheckGeofenceResponse, error) {
	return nil, status.Error(codes.Unimplemented, "spot coordinates not yet available")
}

