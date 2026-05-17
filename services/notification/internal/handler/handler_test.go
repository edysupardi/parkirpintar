package handler_test

import (
	"context"
	"errors"
	"testing"

	notificationv1 "github.com/edysupardi/parkirpintar/gen/notification/v1"
	"github.com/edysupardi/parkirpintar/pkg/logger"
	"github.com/edysupardi/parkirpintar/services/notification/internal/handler"
	"github.com/edysupardi/parkirpintar/services/notification/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPush struct {
	err error
}

func (m *mockPush) Send(_ context.Context, _, _, _ string, _ map[string]string) error {
	return m.err
}

type mockEmail struct {
	err error
}

func (m *mockEmail) Send(_ context.Context, _, _, _ string) error {
	return m.err
}

type mockRepo struct {
	logs    []repository.NotificationLog
	total   int32
	lastLog repository.NotificationLog
	err     error
}

func (m *mockRepo) Insert(_ context.Context, log repository.NotificationLog) error {
	m.lastLog = log
	m.logs = append(m.logs, log)
	return m.err
}

func (m *mockRepo) GetByDriverID(_ context.Context, _ string, _, _ int32) ([]repository.NotificationLog, int32, error) {
	return m.logs, m.total, m.err
}

func newHandler(push *mockPush, email *mockEmail, repo handler.NotificationRepo) *handler.NotificationHandler {
	log := logger.New(logger.Config{Service: "test", Level: "error"})
	return handler.NewWithRepo(push, email, repo, log)
}

func TestSendNotification_Push_Success(t *testing.T) {
	repo := &mockRepo{}
	h := newHandler(&mockPush{}, &mockEmail{}, repo)

	resp, err := h.SendNotification(context.Background(), &notificationv1.SendNotificationRequest{
		DriverId:   "drv-001",
		Channel:    notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_PUSH,
		TemplateId: notificationv1.TemplateID_TEMPLATE_ID_CHECK_IN_SUCCESS,
	})

	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.NotificationId)
	assert.Equal(t, "sent", repo.lastLog.Status)
	assert.Equal(t, "drv-001", repo.lastLog.DriverID)
}

func TestSendNotification_Push_Failure_LogsPersisted(t *testing.T) {
	repo := &mockRepo{}
	h := newHandler(&mockPush{err: errors.New("fcm down")}, &mockEmail{}, repo)

	resp, err := h.SendNotification(context.Background(), &notificationv1.SendNotificationRequest{
		DriverId:   "drv-001",
		Channel:    notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_PUSH,
		TemplateId: notificationv1.TemplateID_TEMPLATE_ID_PAYMENT_FAILED,
	})

	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "fcm down", resp.ErrorMessage)
	assert.Equal(t, "failed", repo.lastLog.Status)
	assert.Equal(t, "fcm down", repo.lastLog.ErrorMessage)
}

func TestSendNotification_Email_Success(t *testing.T) {
	repo := &mockRepo{}
	h := newHandler(&mockPush{}, &mockEmail{}, repo)

	resp, err := h.SendNotification(context.Background(), &notificationv1.SendNotificationRequest{
		DriverId:   "drv-002",
		Channel:    notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL,
		TemplateId: notificationv1.TemplateID_TEMPLATE_ID_INVOICE_GENERATED,
		Data:       map[string]string{"total": "15000"},
	})

	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "sent", repo.lastLog.Status)
	assert.Contains(t, repo.lastLog.Title, "Invoice")
}

func TestSendNotification_Both_PushFails_EmailSucceeds(t *testing.T) {
	repo := &mockRepo{}
	h := newHandler(&mockPush{err: errors.New("push err")}, &mockEmail{}, repo)

	resp, err := h.SendNotification(context.Background(), &notificationv1.SendNotificationRequest{
		DriverId:   "drv-003",
		Channel:    notificationv1.NotificationChannel_NOTIFICATION_CHANNEL_BOTH,
		TemplateId: notificationv1.TemplateID_TEMPLATE_ID_RESERVATION_CONFIRMED,
		Data:       map[string]string{"spot": "5", "floor": "2"},
	})

	require.NoError(t, err)
	assert.True(t, resp.Success)
	assert.Equal(t, "sent", repo.lastLog.Status)
}

func TestGetNotificationHistory_Success(t *testing.T) {
	repo := &mockRepo{
		logs: []repository.NotificationLog{
			{ID: "n1", DriverID: "drv-001", Channel: "NOTIFICATION_CHANNEL_PUSH", Status: "sent"},
			{ID: "n2", DriverID: "drv-001", Channel: "NOTIFICATION_CHANNEL_EMAIL", Status: "failed", ErrorMessage: "bounce"},
		},
		total: 2,
	}
	h := newHandler(&mockPush{}, &mockEmail{}, repo)

	resp, err := h.GetNotificationHistory(context.Background(), &notificationv1.GetNotificationHistoryRequest{
		DriverId: "drv-001",
		Limit:    10,
	})

	require.NoError(t, err)
	assert.Equal(t, int32(2), resp.Total)
	assert.Len(t, resp.Records, 2)
	assert.Equal(t, "n1", resp.Records[0].NotificationId)
}

func TestGetNotificationHistory_EmptyDriverID(t *testing.T) {
	h := newHandler(&mockPush{}, &mockEmail{}, &mockRepo{})

	_, err := h.GetNotificationHistory(context.Background(), &notificationv1.GetNotificationHistoryRequest{
		DriverId: "",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "driver_id is required")
}
