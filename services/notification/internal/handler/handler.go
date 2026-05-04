package handler

import (
	"context"
	"fmt"

	notificationv1 "github.com/edysupardi/parkirpintar/gen/notification/v1"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/notification/internal/domain"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type NotificationHandler struct {
	notificationv1.UnimplementedNotificationServiceServer
	push  domain.PushProvider
	email domain.EmailProvider
	log   logger.Logger
}

func New(push domain.PushProvider, email domain.EmailProvider, log logger.Logger) *NotificationHandler {
	return &NotificationHandler{push: push, email: email, log: log}
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

	if sendErr != nil {
		return &notificationv1.SendNotificationResponse{
			Success:      false,
			NotificationId: notifID,
			ErrorMessage: sendErr.Error(),
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
		TotalSent:       int32(len(req.DriverIds) - len(failed)),
		TotalFailed:     int32(len(failed)),
		FailedDriverIds: failed,
	}, nil
}

func (h *NotificationHandler) GetNotificationHistory(ctx context.Context, req *notificationv1.GetNotificationHistoryRequest) (*notificationv1.GetNotificationHistoryResponse, error) {
	return nil, status.Error(codes.Unimplemented, "history not persisted in stub mode")
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

// ensure timestamppb is used (for future history implementation)
var _ = timestamppb.Now
