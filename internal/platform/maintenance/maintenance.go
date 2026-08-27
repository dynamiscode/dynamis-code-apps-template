package maintenance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

const (
	transientRetention = 30 * 24 * time.Hour
	historyRetention   = 365 * 24 * time.Hour
	replayRetention    = 7 * 24 * time.Hour
)

type Result struct {
	AuditEvents        int64 `json:"auditEvents"`
	Sessions           int64 `json:"sessions"`
	Invitations        int64 `json:"invitations"`
	APITokens          int64 `json:"apiTokens"`
	OIDCTransactions   int64 `json:"oidcTransactions"`
	EmailVerifications int64 `json:"emailVerifications"`
	PasswordResets     int64 `json:"passwordResets"`
	Idempotency        int64 `json:"idempotencyRecords"`
	RealtimeReplay     int64 `json:"realtimeReplay"`
	Notifications      int64 `json:"notifications"`
}

func Run(
	ctx context.Context,
	db *sql.DB,
	driver config.DatabaseDriver,
	now time.Time,
	auditRetention time.Duration,
) (Result, error) {
	now = now.UTC()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, err
	}
	defer tx.Rollback()

	result := Result{}
	deletions := []struct {
		query string
		args  []any
		count *int64
	}{
		{"DELETE FROM idempotency_records WHERE expires_at <= ?", []any{stamp(now)}, &result.Idempotency},
		{"DELETE FROM oidc_transactions WHERE expires_at <= ?", []any{stamp(now)}, &result.OIDCTransactions},
		{`DELETE FROM email_verifications WHERE expires_at <= ? OR
			(consumed_at IS NOT NULL AND consumed_at <= ?)`, []any{stamp(now), stamp(now.Add(-transientRetention))}, &result.EmailVerifications},
		{`DELETE FROM password_resets WHERE expires_at <= ? OR
			(consumed_at IS NOT NULL AND consumed_at <= ?)`, []any{stamp(now), stamp(now.Add(-transientRetention))}, &result.PasswordResets},
		{`DELETE FROM sessions WHERE expires_at <= ? OR
			(revoked_at IS NOT NULL AND revoked_at <= ?)`, []any{stamp(now), stamp(now.Add(-transientRetention))}, &result.Sessions},
		{`DELETE FROM invitations WHERE created_at <= ? AND
			(accepted_at IS NOT NULL OR expired_at IS NOT NULL OR revoked_at IS NOT NULL)`, []any{stamp(now.Add(-historyRetention))}, &result.Invitations},
		{`DELETE FROM api_tokens WHERE created_at <= ? AND
			(revoked_at IS NOT NULL OR (expires_at IS NOT NULL AND expires_at <= ?))`, []any{stamp(now.Add(-historyRetention)), stamp(now)}, &result.APITokens},
		{"DELETE FROM item_events WHERE occurred_at <= ?", []any{stamp(now.Add(-replayRetention))}, &result.RealtimeReplay},
		{"DELETE FROM notifications WHERE created_at <= ?", []any{stamp(now.Add(-historyRetention))}, &result.Notifications},
		{"DELETE FROM audit_events WHERE created_at <= ?", []any{stamp(now.Add(-auditRetention))}, &result.AuditEvents},
	}
	for _, deletion := range deletions {
		execResult, err := tx.ExecContext(
			ctx, database.Rebind(driver, deletion.query), deletion.args...,
		)
		if err != nil {
			return Result{}, err
		}
		*deletion.count, err = execResult.RowsAffected()
		if err != nil {
			return Result{}, err
		}
	}
	metadata, err := json.Marshal(result)
	if err != nil {
		return Result{}, err
	}
	eventID, err := id.New()
	if err != nil {
		return Result{}, err
	}
	_, err = tx.ExecContext(ctx, database.Rebind(driver, `
		INSERT INTO audit_events (
			id, event_type, actor_user_id, auth_method, workspace_id,
			target_type, target_id, action, outcome, request_id,
			source_address, metadata, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), eventID, "maintenance.retention_pruned", nil, "system", nil,
		"instance", nil, "maintenance.prune", "success", nil, nil,
		string(metadata), stamp(now))
	if err != nil {
		return Result{}, fmt.Errorf("record retention audit: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Result{}, err
	}
	return result, nil
}

func stamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}
