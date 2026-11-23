package intake

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode/utf8"

	"example.com/dynamis-code/apps-template/internal/clients"
	"example.com/dynamis-code/apps-template/internal/engagement"
	"example.com/dynamis-code/apps-template/internal/identity"
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
	return tx.Commit()
}
