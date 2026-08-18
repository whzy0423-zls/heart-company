package sms

import "context"

type Sender interface {
	Send(ctx context.Context, phone, code string) error
}

// Reporter records a successful SMS delivery in an external notification log.
// Reporting is intentionally separate from Sender so a reporting outage never
// changes the SMS provider result.
type Reporter interface {
	Report(ctx context.Context, title, content, messageType, channel string) error
}
