package webhooks

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/jobs"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
)

const (
	maxWebhooks            = 20
	maxDeliveryHistory     = 100
	maxAttempts            = 5
	deliveryRequestTimeout = 10 * time.Second
	initialRetryDelay      = time.Second
	maxWebhookNameLength   = 100
	maxWebhookURLLength    = 2048
	maxWebhookPayloadBytes = 1 << 20
)

var (
	ErrInvalidInput = errors.New("webhook input is invalid")
	ErrNotFound     = errors.New("webhook not found")
	ErrLimit        = errors.New("workspace webhook limit reached")
	ErrSecretKey    = errors.New("webhook secret encryption key is unavailable")
)

var SupportedEvents = []string{"item.created", "item.updated", "item.deleted"}

type CreateInput struct {
	Name   string
	URL    string
	Events []string
}

type Webhook struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Events      []string  `json:"events"`
	CreatedAt   time.Time `json:"createdAt"`
}

type NewWebhook struct {
	Webhook
	Secret string `json:"secret"`
}

type Delivery struct {
	ID             string     `json:"id"`
	WebhookID      string     `json:"webhookId"`
	EventID        string     `json:"eventId"`
	EventType      string     `json:"eventType"`
	AttemptCount   int        `json:"attemptCount"`
	Status         string     `json:"status"`
	NextAttemptAt  *time.Time `json:"nextAttemptAt,omitempty"`
	LastStatusCode int        `json:"lastStatusCode,omitzero"`
	LastError      string     `json:"lastError,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	DeliveredAt    *time.Time `json:"deliveredAt,omitempty"`
}

type Service struct {
	db        *sql.DB
	driver    config.DatabaseDriver
	auth      *identity.Service
	secretKey []byte
	client    *http.Client
	queue     *jobs.Queue
}

func NewService(
	db *sql.DB,
	driver config.DatabaseDriver,
	auth *identity.Service,
	secretKey []byte,
	queue *jobs.Queue,
) *Service {
	return &Service{
		db: db, driver: driver, auth: auth,
		secretKey: append([]byte(nil), secretKey...),
		client:    newHTTPClient(), queue: queue,
	}
}

func (s *Service) Create(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	input CreateInput,
	audit identity.AuditContext,
) (NewWebhook, error) {
	name := strings.TrimSpace(input.Name)
	urlValue, err := validateURL(input.URL)
	if err != nil || name == "" || len(name) > maxWebhookNameLength {
		return NewWebhook{}, ErrInvalidInput
	}
	events, err := validateEvents(input.Events)
	if err != nil || len(s.secretKey) != 32 {
		if len(s.secretKey) != 32 {
			return NewWebhook{}, ErrSecretKey
		}
		return NewWebhook{}, ErrInvalidInput
	}
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.WebhooksManage); err != nil {
		return NewWebhook{}, identity.ErrForbidden
	}
	secret, err := newSecret()
	if err != nil {
		return NewWebhook{}, err
	}
	ciphertext, err := s.encrypt(secret)
	if err != nil {
		return NewWebhook{}, err
	}
	webhookID, err := id.New()
	if err != nil {
		return NewWebhook{}, err
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return NewWebhook{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.WebhooksManage); err != nil {
		return NewWebhook{}, identity.ErrForbidden
	}
	var count int
	if err := s.queryRow(ctx, tx, "SELECT COUNT(*) FROM webhooks WHERE workspace_id = ?", workspaceID).Scan(&count); err != nil {
		return NewWebhook{}, err
	}
	if count >= maxWebhooks {
		return NewWebhook{}, ErrLimit
	}
	if _, err := s.exec(ctx, tx, `
		INSERT INTO webhooks (id, workspace_id, name, url, secret_ciphertext, events, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, webhookID, workspaceID, name, urlValue, ciphertext, encodeEvents(events), stamp(now)); err != nil {
		return NewWebhook{}, err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "webhook.created", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: workspaceID, TargetType: "webhook", TargetID: webhookID,
		Action: "webhook.create", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress,
		Metadata:      metadata(map[string]any{"events": events, "host": parsedHost(urlValue)}), CreatedAt: now,
	}); err != nil {
		return NewWebhook{}, err
	}
	if err := tx.Commit(); err != nil {
		return NewWebhook{}, err
	}
	return NewWebhook{Webhook: Webhook{
		ID: webhookID, WorkspaceID: workspaceID, Name: name, URL: urlValue,
		Events: events, CreatedAt: now,
	}, Secret: secret}, nil
}

func (s *Service) List(ctx context.Context, actor identity.Principal, workspaceID string) ([]Webhook, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.WebhooksRead); err != nil {
		return nil, identity.ErrForbidden
	}
	rows, err := s.db.QueryContext(ctx, s.bind(`
		SELECT id, workspace_id, name, url, events, created_at
		FROM webhooks WHERE workspace_id = ? ORDER BY created_at DESC, id DESC
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Webhook, 0)
	for rows.Next() {
		webhook, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, webhook)
	}
	return result, rows.Err()
}

func (s *Service) Delete(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	webhookID string,
	audit identity.AuditContext,
) error {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.WebhooksManage); err != nil {
		return identity.ErrForbidden
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.WebhooksManage); err != nil {
		return identity.ErrForbidden
	}
	result, err := s.exec(ctx, tx, "DELETE FROM webhooks WHERE id = ? AND workspace_id = ?", webhookID, workspaceID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrNotFound
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "webhook.deleted", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: workspaceID, TargetType: "webhook", TargetID: webhookID,
		Action: "webhook.delete", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: time.Now().UTC(),
	}); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) RotateSecret(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	webhookID string,
	audit identity.AuditContext,
) (string, error) {
	if len(s.secretKey) != 32 {
		return "", ErrSecretKey
	}
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.WebhooksManage); err != nil {
		return "", identity.ErrForbidden
	}
	secret, err := newSecret()
	if err != nil {
		return "", err
	}
	ciphertext, err := s.encrypt(secret)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.WebhooksManage); err != nil {
		return "", identity.ErrForbidden
	}
	result, err := s.exec(ctx, tx, "UPDATE webhooks SET secret_ciphertext = ? WHERE id = ? AND workspace_id = ?", ciphertext, webhookID, workspaceID)
	if err != nil {
		return "", err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return "", err
	}
	if changed != 1 {
		return "", ErrNotFound
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "webhook.secret.rotated", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: workspaceID, TargetType: "webhook", TargetID: webhookID,
		Action: "webhook.secret.rotate", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return secret, nil
}

func (s *Service) ListDeliveries(
	ctx context.Context,
	actor identity.Principal,
	workspaceID string,
	webhookID string,
) ([]Delivery, error) {
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.WebhooksRead); err != nil {
		return nil, identity.ErrForbidden
	}
	var exists int
	if err := s.queryRow(ctx, s.db, "SELECT 1 FROM webhooks WHERE id = ? AND workspace_id = ?", webhookID, workspaceID).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, s.bind(`
		SELECT d.id, d.webhook_id, d.event_id, d.event_type, d.attempt_count,
			d.status, d.next_attempt_at, d.last_status_code, d.last_error,
			d.created_at, d.delivered_at
		FROM webhook_deliveries d JOIN webhooks w ON w.id = d.webhook_id
		WHERE d.webhook_id = ? AND w.workspace_id = ?
		ORDER BY d.created_at DESC, d.id DESC LIMIT ?
	`), webhookID, workspaceID, maxDeliveryHistory)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Delivery, 0)
	for rows.Next() {
		var delivery Delivery
		var nextAttempt, delivered sql.NullString
		var lastError sql.NullString
		var lastStatusCode sql.NullInt64
		var created string
		if err := rows.Scan(&delivery.ID, &delivery.WebhookID, &delivery.EventID, &delivery.EventType,
			&delivery.AttemptCount, &delivery.Status, &nextAttempt, &lastStatusCode,
			&lastError, &created, &delivered); err != nil {
			return nil, err
		}
		if lastStatusCode.Valid {
			delivery.LastStatusCode = int(lastStatusCode.Int64)
		}
		var err error
		if delivery.NextAttemptAt, err = optionalTime(nextAttempt); err != nil {
			return nil, err
		}
		if delivery.DeliveredAt, err = optionalTime(delivered); err != nil {
			return nil, err
		}
		if delivery.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, err
		}
		if lastError.Valid {
			delivery.LastError = lastError.String
		}
		result = append(result, delivery)
	}
	return result, rows.Err()
}

// PublishInTx is the item service's durable event boundary. It writes one
// delivery per matching registration before the resource transaction commits.
func (s *Service) PublishInTx(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID string,
	eventType string,
	data []byte,
	occurredAt time.Time,
) error {
	if len(data) == 0 || len(data) > maxWebhookPayloadBytes || !contains(SupportedEvents, eventType) {
		return ErrInvalidInput
	}
	var raw json.RawMessage = data
	if !json.Valid(raw) {
		return ErrInvalidInput
	}
	rows, err := tx.QueryContext(ctx, s.bind("SELECT id, events FROM webhooks WHERE workspace_id = ?"), workspaceID)
	if err != nil {
		return err
	}
	type target struct{ id, events string }
	targets := make([]target, 0)
	for rows.Next() {
		var value target
		if err := rows.Scan(&value.id, &value.events); err != nil {
			rows.Close()
			return err
		}
		if contains(decodeEvents(value.events), eventType) {
			targets = append(targets, value)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	eventID, err := id.New()
	if err != nil {
		return err
	}
	for _, target := range targets {
		envelope, err := json.Marshal(map[string]any{
			"id": eventID, "type": eventType, "timestamp": occurredAt.UTC(),
			"workspaceId": workspaceID, "data": raw,
		})
		if err != nil {
			return err
		}
		deliveryID, err := id.New()
		if err != nil {
			return err
		}
		result, err := s.exec(ctx, tx, `
			INSERT INTO webhook_deliveries (
				id, webhook_id, event_id, event_type, payload, attempt_count,
				status, next_attempt_at, last_status_code, last_error, created_at
			) VALUES (?, ?, ?, ?, ?, 0, 'pending', ?, NULL, NULL, ?)
			ON CONFLICT (webhook_id, event_id) DO NOTHING
		`, deliveryID, target.id, eventID, eventType, string(envelope), stamp(occurredAt), stamp(occurredAt))
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed == 1 {
			if s.queue == nil {
				return errors.New("webhook job queue is unavailable")
			}
			jobPayload, err := json.Marshal(map[string]string{"deliveryId": deliveryID})
			if err != nil {
				return err
			}
			if err := s.queue.EnqueueTx(ctx, tx, workspaceID, JobKind, deliveryID, string(jobPayload), occurredAt); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) DeliverPending(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > maxDeliveryHistory {
		return 0, ErrInvalidInput
	}
	if s.queue == nil {
		return 0, errors.New("webhook job queue is unavailable")
	}
	return s.queue.Process(ctx, limit)
}

const JobKind = "webhook.delivery"

func (s *Service) HandleJob(ctx context.Context, job jobs.Job) error {
	var payload struct {
		DeliveryID string `json:"deliveryId"`
	}
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil || payload.DeliveryID == "" {
		return jobs.Failure{Category: "payload-invalid"}
	}
	return s.deliverOne(ctx, job.WorkspaceID, payload.DeliveryID, job.AttemptCount)
}

func (s *Service) deliverOne(ctx context.Context, workspaceID, deliveryID string, jobAttempt int) error {
	now := time.Now().UTC()
	var webhookID, eventID, eventType, payload, urlValue, encrypted string
	var attempt int
	err := s.queryRow(ctx, s.db, `
		SELECT d.id, d.webhook_id, d.event_id, d.event_type, d.payload,
			d.attempt_count, w.url, w.secret_ciphertext
		FROM webhook_deliveries d JOIN webhooks w ON w.id = d.webhook_id
		WHERE d.id = ? AND d.status = 'pending' AND w.workspace_id = ?
	`, deliveryID, workspaceID).Scan(&deliveryID, &webhookID, &eventID, &eventType, &payload, &attempt, &urlValue, &encrypted)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return jobs.Failure{Category: "storage-error"}
	}
	attempt++
	if attempt < jobAttempt {
		attempt = jobAttempt
	}
	if _, err := s.exec(ctx, s.db, "UPDATE webhook_deliveries SET attempt_count = ? WHERE id = ? AND status = 'pending'", attempt, deliveryID); err != nil {
		return jobs.Failure{Category: "storage-error"}
	}
	secret, err := s.decrypt(encrypted)
	if err != nil {
		return s.deliveryFailure(ctx, deliveryID, attempt, 0, "secret-unavailable", now)
	}
	timestamp := strconv.FormatInt(now.Unix(), 10)
	message := eventID + "." + timestamp + "." + payload
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(message))
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, urlValue, strings.NewReader(payload))
	if err != nil {
		return s.deliveryFailure(ctx, deliveryID, attempt, 0, "request-invalid", now)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Dynamis-Code-Webhooks/1")
	request.Header.Set("Webhook-Id", eventID)
	request.Header.Set("Webhook-Timestamp", timestamp)
	request.Header.Set("Webhook-Signature", "v1,"+base64.RawStdEncoding.EncodeToString(mac.Sum(nil)))
	response, err := s.client.Do(request)
	if err != nil {
		return s.deliveryFailure(ctx, deliveryID, attempt, 0, "network-error", now)
	}
	_, _ = io.CopyN(io.Discard, response.Body, 4096)
	_ = response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return s.deliveryFailure(ctx, deliveryID, attempt, response.StatusCode, "remote-status", now)
	}
	if _, err := s.exec(ctx, s.db, `
		UPDATE webhook_deliveries SET status = 'delivered', delivered_at = ?,
		next_attempt_at = NULL, last_status_code = ?, last_error = NULL WHERE id = ? AND status = 'pending'
	`, stamp(now), response.StatusCode, deliveryID); err != nil {
		return jobs.Failure{Category: "storage-error"}
	}
	telemetry.RecordWebhookDelivery(ctx, "delivered")
	return nil
}

func (s *Service) deliveryFailure(
	ctx context.Context,
	deliveryID string,
	attempt, statusCode int,
	reason string,
	now time.Time,
) error {
	status := "pending"
	var next any = stamp(now.Add(retryDelay(attempt)))
	if attempt >= maxAttempts {
		status, next = "failed", nil
	}
	if _, err := s.exec(ctx, s.db, `
		UPDATE webhook_deliveries SET status = ?, next_attempt_at = ?,
		last_status_code = ?, last_error = ? WHERE id = ? AND status = 'pending'
	`, status, next, nullableStatus(statusCode), reason, deliveryID); err != nil {
		return jobs.Failure{Category: "storage-error"}
	}
	telemetry.RecordWebhookDelivery(ctx, status)
	return jobs.Failure{Category: reason}
}

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: deliveryRequestTimeout, Transport: &http.Transport{
		Proxy: nil, DialContext: safeDialContext,
	}, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

func safeDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	allowLoopback := isLoopbackHost(host)
	for _, ip := range ips {
		if restrictedIP(ip) && !allowLoopback {
			continue
		}
		return (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	return nil, errors.New("webhook endpoint resolves to a restricted address")
}

func restrictedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast()
}

func validateURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > maxWebhookURLLength {
		return "", ErrInvalidInput
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalidInput
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && isLoopbackHost(parsed.Hostname())) {
		return "", ErrInvalidInput
	}
	if ip := net.ParseIP(parsed.Hostname()); ip != nil && restrictedIP(ip) && !ip.IsLoopback() {
		return "", ErrInvalidInput
	}
	return value, nil
}

func validateEvents(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > len(SupportedEvents) {
		return nil, ErrInvalidInput
	}
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] || !contains(SupportedEvents, value) {
			return nil, ErrInvalidInput
		}
		seen[value] = true
		result = append(result, value)
	}
	return result, nil
}

func encodeEvents(values []string) string {
	encoded, _ := json.Marshal(values)
	return string(encoded)
}

func decodeEvents(value string) []string {
	var values []string
	if json.Unmarshal([]byte(value), &values) != nil {
		return nil
	}
	return values
}

func newSecret() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return "whsec_" + base64.RawURLEncoding.EncodeToString(value), nil
}

func (s *Service) encrypt(secret string) (string, error) {
	block, err := aes.NewCipher(s.secretKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nil, nonce, []byte(secret), nil)
	return base64.RawStdEncoding.EncodeToString(append(nonce, sealed...)), nil
}

func (s *Service) decrypt(value string) (string, error) {
	block, err := aes.NewCipher(s.secretKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	encoded, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || len(encoded) < gcm.NonceSize() {
		return "", ErrSecretKey
	}
	plain, err := gcm.Open(nil, encoded[:gcm.NonceSize()], encoded[gcm.NonceSize():], nil)
	if err != nil {
		return "", ErrSecretKey
	}
	return string(plain), nil
}

func (s *Service) exec(ctx context.Context, executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, query string, args ...any) (sql.Result, error) {
	return executor.ExecContext(ctx, s.bind(query), args...)
}

func (s *Service) queryRow(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query string, args ...any) *sql.Row {
	return queryer.QueryRowContext(ctx, s.bind(query), args...)
}

func (s *Service) bind(query string) string { return database.Rebind(s.driver, query) }

func scanWebhook(row interface{ Scan(...any) error }) (Webhook, error) {
	var webhook Webhook
	var events, created string
	if err := row.Scan(&webhook.ID, &webhook.WorkspaceID, &webhook.Name, &webhook.URL, &events, &created); err != nil {
		return Webhook{}, err
	}
	var err error
	webhook.Events = decodeEvents(events)
	webhook.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return webhook, err
}

func optionalTime(value sql.NullString) (*time.Time, error) {
	if !value.Valid || value.String == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value.String)
	return &parsed, err
}

func stamp(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func retryDelay(attempt int) time.Duration {
	return initialRetryDelay * time.Duration(1<<(attempt-1))
}

func nullableStatus(value int) any {
	if value == 0 {
		return nil
	}
	return value
}

func parsedHost(value string) string {
	parsed, _ := url.Parse(value)
	return parsed.Hostname()
}

func isLoopbackHost(host string) bool {
	return strings.EqualFold(host, "localhost") || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func metadata(value map[string]any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
