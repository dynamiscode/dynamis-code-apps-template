package mail

import (
	"context"
	"errors"
)

// ErrNotConfigured means no sender exists for the workspace or the instance.
var ErrNotConfigured = errors.New("email delivery is not configured")

// WorkspaceSender routes invitation mail through a workspace's own SMTP settings
// when present, and through the instance sender otherwise.
type WorkspaceSender interface {
	Sender
	SendFor(ctx context.Context, workspaceID, recipient, subject, body string) error
	// Available reports whether instance-level (account) delivery is configured.
	Available() bool
}

// SendFor delivers through the workspace router when one is wired, else the plain sender.
func SendFor(ctx context.Context, sender Sender, workspaceID, recipient, subject, body string) error {
	if router, ok := sender.(WorkspaceSender); ok {
		return router.SendFor(ctx, workspaceID, recipient, subject, body)
	}
	if sender == nil {
		return ErrNotConfigured
	}
	return sender.Send(ctx, recipient, subject, body)
}

// Enabled reports whether account email (verification, password reset) can be sent.
func Enabled(sender Sender) bool {
	if sender == nil {
		return false
	}
	if router, ok := sender.(WorkspaceSender); ok {
		return router.Available()
	}
	return true
}
