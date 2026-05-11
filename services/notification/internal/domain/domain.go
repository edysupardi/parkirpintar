package domain

import "context"

type Channel string
type Status string

const (
	ChannelPush  Channel = "push"
	ChannelEmail Channel = "email"
	ChannelBoth  Channel = "both"

	StatusSent    Status = "sent"
	StatusFailed  Status = "failed"
	StatusSkipped Status = "skipped"
)

type Notification struct {
	DriverID   string
	Channel    Channel
	TemplateID string
	Data       map[string]string
	Priority   string
}

type PushProvider interface {
	Send(ctx context.Context, driverID, title, body string, data map[string]string) error
}

type EmailProvider interface {
	Send(ctx context.Context, to, subject, body string) error
}
