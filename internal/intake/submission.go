package intake

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/jobs"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

type Submission struct {
	DisplayName, Email, Phone, Notes string
	Consent                          bool
}

func (s *Service) PublicForm(ctx context.Context, token string) (string, error) {
	if token == "" {
		return "", ErrNotFound
	}
	var consent string
	err := s.db.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT consent_text FROM public_intake_links l WHERE l.token_hash = ? AND EXISTS (SELECT 1 FROM engagement_stages s WHERE s.workspace_id = l.workspace_id AND s.kind = 'open' AND s.archived_at IS NULL)`), hashToken(token)).Scan(&consent)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return consent, err
}

func (s *Service) Submit(ctx context.Context, token string, input Submission, audit identity.AuditContext) error {
	if !input.Consent || strings.TrimSpace(input.DisplayName) == "" || (strings.TrimSpace(input.Email) == "" && strings.TrimSpace(input.Phone) == "") || !utf8.ValidString(input.Notes) || utf8.RuneCountInString(input.Notes) > 2000 {
		return ErrInvalidInput
	}
	if token == "" {
		return ErrNotFound
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var workspace identity.WorkspaceID
	err = tx.QueryRowContext(ctx, database.Rebind(s.driver, `SELECT workspace_id FROM public_intake_links WHERE token_hash = ?`), hashToken(token)).Scan(&workspace)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	client, err := s.clients.CreateIntakeInTx(ctx, tx, token, workspace, clients.Input{DisplayName: input.DisplayName, Email: input.Email, Phone: input.Phone, Notes: input.Notes}, audit)
	if errors.Is(err, clients.ErrInvalidInput) {
		return ErrInvalidInput
	}
	if errors.Is(err, identity.ErrForbidden) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	_, err = s.engagement.CreateIntakeInTx(ctx, tx, token, workspace, client.ID, audit)
	if errors.Is(err, engagement.ErrNoOpenStage) || errors.Is(err, identity.ErrForbidden) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `SELECT m.user_id, u.email FROM workspace_members m JOIN users u ON u.id = m.user_id WHERE m.workspace_id = ? AND m.role IN ('owner', 'admin')`), workspace)
	if err != nil {
		return err
	}
	var recipients []identity.UserID
	var emails []string
	for rows.Next() {
		var recipient identity.UserID
		var email string
		if err := rows.Scan(&recipient, &email); err != nil {
			rows.Close()
			return err
		}
		recipients = append(recipients, recipient)
		emails = append(emails, email)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	path := "/workspaces/" + workspace.String() + "/clients/" + client.ID.String()
	for _, recipient := range recipients {
		_, err := s.auth.CreateNotificationInTx(ctx, tx, identity.Principal{AuthMethod: "system"}, identity.NotificationInput{
			RecipientUserID: recipient, WorkspaceID: workspace, NotificationType: "public_intake",
			Title: "New intake submission", Body: path,
		}, audit)
		if err != nil {
			return err
		}
	}
	if s.queue != nil && len(emails) > 0 {
		if err := jobs.EnqueueEmailTx(ctx, s.queue, tx, workspace, int64(client.ID), jobs.Email{
			Recipients: emails, Subject: "New intake submission", Body: s.publicURL + path,
		}, time.Now().UTC()); err != nil {
			return err
		}
	}
	return tx.Commit()
}
