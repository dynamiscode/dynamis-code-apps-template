package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
)

const EmailJobKind = "email.delivery"

type Email struct {
	Recipient string `json:"recipient"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

func EnqueueEmailTx(ctx context.Context, queue *Queue, tx *sql.Tx, workspaceID identity.WorkspaceID, key int64, message Email, availableAt time.Time) error {
	if _, err := mail.ParseAddress(message.Recipient); err != nil || strings.ContainsAny(message.Recipient+message.Subject, "\r\n") || strings.TrimSpace(message.Subject) == "" || strings.TrimSpace(message.Body) == "" {
		return errors.New("email job input is invalid")
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return queue.EnqueueTx(ctx, tx, workspaceID, EmailJobKind, key, string(payload), availableAt)
}

func EmailHandler(sender appmail.Sender) Handler {
	return func(ctx context.Context, job Job) error {
		var message Email
		if err := json.Unmarshal([]byte(job.Payload), &message); err != nil {
			return Failure{Category: "email-invalid"}
		}
		if err := appmail.SendFor(ctx, sender, job.WorkspaceID.String(), message.Recipient, message.Subject, message.Body); err != nil {
			return Failure{Category: "email-delivery"}
		}
		return nil
	}
}
