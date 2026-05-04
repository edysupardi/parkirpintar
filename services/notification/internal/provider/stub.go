package provider

import (
	"context"

	"github.com/edysupardi/parkirpintar/pkg/logger"
)

// StubPushProvider logs instead of sending real FCM notifications.
type StubPushProvider struct {
	log logger.Logger
}

func NewStubPush(log logger.Logger) *StubPushProvider {
	return &StubPushProvider{log: log}
}

func (p *StubPushProvider) Send(ctx context.Context, driverID, title, body string, data map[string]string) error {
	p.log.Info(ctx).
		Str("driver_id", driverID).
		Str("title", title).
		Str("body", body).
		Msg("[stub] push notification sent")
	return nil
}

// StubEmailProvider logs instead of sending real SES emails.
type StubEmailProvider struct {
	log logger.Logger
}

func NewStubEmail(log logger.Logger) *StubEmailProvider {
	return &StubEmailProvider{log: log}
}

func (p *StubEmailProvider) Send(ctx context.Context, to, subject, body string) error {
	p.log.Info(ctx).
		Str("to", to).
		Str("subject", subject).
		Msg("[stub] email sent")
	return nil
}
