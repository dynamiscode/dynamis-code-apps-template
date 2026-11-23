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
	"example.com/dynamis-code/apps-template/internal/platform/database"
	appmail "example.com/dynamis-code/apps-template/internal/platform/mail"
)

const EmailJobKind = "email.delivery"

type Email struct {
	Recipient  string   `json:"recipient"`
	Recipients []string `json:"recipients,omitempty"`
	Subject    string   `json:"subject"`
	Body       string   `json:"body"`
}

func EnqueueEmailTx(ctx context.Context, queue *Queue, tx *sql.Tx, workspaceID identity.WorkspaceID, key int64, message Email, availableAt time.Time) error {
	recipients := message.Recipients
	if message.Recipient != "" {
		recipients = append(recipients, message.Recipient)
	}
	if len(recipients) == 0 || strings.ContainsAny(message.Subject, "\r\n") || strings.TrimSpace(message.Subject) == "" || strings.TrimSpace(message.Body) == "" {
		return errors.New("email job input is invalid")
	}
	for _, recipient := range recipients {
		if _, err := mail.ParseAddress(recipient); err != nil || strings.ContainsAny(recipient, "\r\n") {
			return errors.New("email job input is invalid")
		}
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return queue.EnqueueTx(ctx, tx, workspaceID, EmailJobKind, key, string(payload), availableAt)
}

func EmailHandler(queue *Queue, sender appmail.Sender) Handler {
	return func(ctx context.Context, job Job) error {
		var message Email
		if err := json.Unmarshal([]byte(job.Payload), &message); err != nil {
			return Failure{Category: "email-invalid"}
		}
		recipients := message.Recipients
		if message.Recipient != "" {
			recipients = append(recipients, message.Recipient)
		}
		if len(recipients) == 0 {
			return Failure{Category: "email-invalid"}
		}
		message.Recipient = ""
		message.Recipients = recipients
		for i := 0; i < len(message.Recipients); {
			recipient := message.Recipients[i]
			if err := appmail.SendFor(ctx, sender, job.WorkspaceID.String(), recipient, message.Subject, message.Body); err != nil {
				i++
				continue
			}
			message.Recipients = append(message.Recipients[:i], message.Recipients[i+1:]...)
			if len(message.Recipients) == 0 {
				return nil
			}
			payload, err := json.Marshal(message)
			if err != nil {
				return Failure{Category: "email-invalid"}
			}
			result, err := queue.db.ExecContext(ctx, database.Rebind(queue.driver, `UPDATE background_jobs SET payload = ? WHERE id = ? AND status = 'running' AND lease_token = ?`), string(payload), job.ID, job.LeaseToken)
			if err != nil {
				return Failure{Category: "email-delivery"}
			}
			if n, err := result.RowsAffected(); err != nil || n != 1 {
				return Failure{Category: "email-delivery"}
			}
		}
		return Failure{Category: "email-delivery"}
	}
}
