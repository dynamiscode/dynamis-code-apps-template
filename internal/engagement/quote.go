package engagement

import (
	"context"
	"database/sql"
	"regexp"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/database"
)

var quoteAmount = regexp.MustCompile(`^[0-9]{1,12}(\.[0-9]{1,2})?$`)

func InvalidQuoteFields(quote Quote) (status, amount, date bool) {
	switch quote.Status {
	case "not_sent", "sent", "pending", "accepted", "declined":
	default:
		status = true
	}
	amount = quote.Amount != "" && !quoteAmount.MatchString(quote.Amount)
	if quote.DateSent != "" {
		_, err := time.Parse("2006-01-02", quote.DateSent)
		date = err != nil
	}
	return
}

func (s *Service) SetQuote(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID, engagementID ID, quote Quote, audit identity.AuditContext) error {
	status, amount, date := InvalidQuoteFields(quote)
	if status || amount || date {
		return ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return identity.ErrForbidden
	}
	result, err := tx.ExecContext(ctx, database.Rebind(s.driver, `UPDATE engagements SET quote_status = ?, quote_amount = ?, quote_date_sent = ? WHERE workspace_id = ? AND id = ?`), quote.Status, quote.Amount, quote.DateSent, workspaceID, engagementID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{EventType: "engagement.quote_updated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod, WorkspaceID: workspaceID, TargetType: "engagement", TargetKey: int64(engagementID), Action: "engagement.quote.update", Outcome: "success", RequestID: audit.RequestID, SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC()}); err != nil {
		return err
	}
	return tx.Commit()
}
