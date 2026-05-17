package handler

import (
	"context"
	"fmt"
	"time"

	notificationv1 "github.com/edysupardi/parkirpintar/gen/notification/v1"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/notification/internal/domain"
	"github.com/edysupardi/parkirpintar/services/notification/internal/repository"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type NotificationRepo interface {
	Insert(ctx context.Context, log repository.NotificationLog) error
	GetByDriverID(ctx context.Context, driverID string, limit, offset int32) ([]repository.NotificationLog, int32, error)
}

type NotificationHandler struct {
	notificationv1.UnimplementedNotificationServiceServer
	push  domain.PushProvider
	email domain.EmailProvider
	repo  NotificationRepo
	log   logger.Logger
}

func New(push domain.PushProvider, email domain.EmailProvider, repo *repository.Repository, log logger.Logger) *NotificationHandler {
	return &NotificationHandler{push: push, email: email, repo: repo, log: log}
}

func NewWithRepo(push domain.PushProvider, email domain.EmailProvider, repo NotificationRepo, log logger.Logger) *NotificationHandler {
	return &NotificationHandler{push: push, email: email, repo: repo, log: log}
}

func (h *NotificationHandler) SendNotification(ctx context.Context, req *notificationv1.SendNotificationRequest) (*notificationv1.SendNotificationResponse, error) {
	notifID := uuid.New().String()
	title, body := templateContent(req.TemplateId, req.Data)

	var sendErr error
	switch req.Channel {
	case notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_PUSH:
		sendErr = h.push.Send(ctx, req.DriverId, title, body, req.Data)
	case notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL:
		sendErr = h.email.Send(ctx, req.DriverId, title, body)
	case notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_BOTH:
		if err := h.push.Send(ctx, req.DriverId, title, body, req.Data); err != nil {
			h.log.Warn(ctx).Err(err).Msg("push failed")
		}
		sendErr = h.email.Send(ctx, req.DriverId, title, body)
	}

	logEntry := repository.NotificationLog{
		ID:         notifID,
		DriverID:   req.DriverId,
		Channel:    req.Channel.String(),
		TemplateID: req.TemplateId.String(),
		Title:      title,
		Body:       body,
		Status:     "sent",
		SentAt:     time.Now(),
	}

	if sendErr != nil {
		logEntry.Status = "failed"
		logEntry.ErrorMessage = sendErr.Error()
	}

	if h.repo != nil {
		if err := h.repo.Insert(ctx, logEntry); err != nil {
			h.log.Warn(ctx).Err(err).Msg("failed to persist notification log")
		}
	}

	if sendErr != nil {
		return &notificationv1.SendNotificationResponse{
			Success:        false,
			NotificationId: notifID,
			ErrorMessage:   sendErr.Error(),
		}, nil
	}

	return &notificationv1.SendNotificationResponse{
		Success:        true,
		NotificationId: notifID,
	}, nil
}

func (h *NotificationHandler) BroadcastNotification(ctx context.Context, req *notificationv1.BroadcastNotificationRequest) (*notificationv1.BroadcastNotificationResponse, error) {
	var failed []string
	for _, driverID := range req.DriverIds {
		_, err := h.SendNotification(ctx, &notificationv1.SendNotificationRequest{
			DriverId:   driverID,
			Channel:    req.Channel,
			TemplateId: req.TemplateId,
			Data:       req.Data,
		})
		if err != nil {
			failed = append(failed, driverID)
		}
	}
	return &notificationv1.BroadcastNotificationResponse{
		TotalSent:       int32(len(req.DriverIds) - len(failed)), // #nosec G115 — bounded by request size
		TotalFailed:     int32(len(failed)),                      // #nosec G115 — bounded by request size
		FailedDriverIds: failed,
	}, nil
}

func (h *NotificationHandler) GetNotificationHistory(ctx context.Context, req *notificationv1.GetNotificationHistoryRequest) (*notificationv1.GetNotificationHistoryResponse, error) {
	if req.DriverId == "" {
		return nil, status.Error(codes.InvalidArgument, "driver_id is required")
	}
	if h.repo == nil {
		return nil, status.Error(codes.Unavailable, "notification history not available")
	}

	logs, total, err := h.repo.GetByDriverID(ctx, req.DriverId, req.Limit, req.Offset)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("query history: %v", err))
	}

	records := make([]*notificationv1.NotificationRecord, 0, len(logs))
	for _, l := range logs {
		records = append(records, &notificationv1.NotificationRecord{
			NotificationId: l.ID,
			DriverId:       l.DriverID,
			Channel:        channelFromString(l.Channel),
			TemplateId:     templateFromString(l.TemplateID),
			Status:         statusFromString(l.Status),
			ErrorMessage:   l.ErrorMessage,
			SentAt:         timestamppb.New(l.SentAt),
		})
	}

	return &notificationv1.GetNotificationHistoryResponse{
		Records: records,
		Total:   total,
	}, nil
}

func channelFromString(s string) notificationv1.NotificationChannel {
	if v, ok := notificationv1.NotificationChannel_value[s]; ok {
		return notificationv1.NotificationChannel(v)
	}
	return notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_UNSPECIFIED
}

func templateFromString(s string) notificationv1.TemplateID {
	if v, ok := notificationv1.TemplateID_value[s]; ok {
		return notificationv1.TemplateID(v)
	}
	return notificationv1.TemplateID_TEMPLATE_ID_UNSPECIFIED
}

func statusFromString(s string) notificationv1.NotificationStatus {
	switch s {
	case "sent":
		return notificationv1.NotificationStatus_NOTIFICATION_STATUS_SENT
	case "failed":
		return notificationv1.NotificationStatus_NOTIFICATION_STATUS_FAILED
	default:
		return notificationv1.NotificationStatus_NOTIFICATION_STATUS_UNSPECIFIED
	}
}

func templateContent(tmpl notificationv1.TemplateID, data map[string]string) (title, body string) {
	switch tmpl {
	case notificationv1.TemplateID_TEMPLATE_ID_RESERVATION_CONFIRMED:
		return "Reservasi Dikonfirmasi", fmt.Sprintf("Spot %s lantai %s telah dikonfirmasi. Berlaku 1 jam.", data["spot"], data["floor"])
	case notificationv1.TemplateID_TEMPLATE_ID_RESERVATION_EXPIRED:
		return "Reservasi Kedaluwarsa", "Reservasi Anda telah kedaluwarsa karena tidak check-in dalam 1 jam."
	case notificationv1.TemplateID_TEMPLATE_ID_RESERVATION_CANCELLED:
		return "Reservasi Dibatalkan", fmt.Sprintf("Reservasi dibatalkan. Biaya: Rp %s", data["fee"])
	case notificationv1.TemplateID_TEMPLATE_ID_CHECK_IN_SUCCESS:
		return "Check-in Berhasil", "Sesi parkir Anda telah dimulai."
	case notificationv1.TemplateID_TEMPLATE_ID_CHECK_OUT_SUCCESS:
		return "Check-out Berhasil", fmt.Sprintf("Sesi parkir selesai. Total: Rp %s", data["total"])
	case notificationv1.TemplateID_TEMPLATE_ID_PAYMENT_SUCCESS:
		return "Pembayaran Berhasil", fmt.Sprintf("Pembayaran Rp %s berhasil. Terima kasih!", data["amount"])
	case notificationv1.TemplateID_TEMPLATE_ID_PAYMENT_FAILED:
		return "Pembayaran Gagal", "Pembayaran gagal. Silakan coba lagi."
	case notificationv1.TemplateID_TEMPLATE_ID_INVOICE_GENERATED:
		return "Invoice Tersedia", fmt.Sprintf("Invoice Rp %s siap dibayar.", data["total"])
	default:
		return "Notifikasi ParkirPintar", data["message"]
	}
}
