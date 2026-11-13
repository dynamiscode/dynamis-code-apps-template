package webhooks

import (
	"context"
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
	"example.com/dynamis-code/apps-template/internal/platform/secretbox"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
)

const (
	maxWebhooks            = 20
	maxDeliveryHistory     = 100
	maxAttempts            = jobs.MaxAttempts
	deliveryRequestTimeout = 10 * time.Second
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
	ID          WebhookID            `json:"id"`
	WorkspaceID identity.WorkspaceID `json:"workspaceId"`
	Name        string               `json:"name"`
	URL         string               `json:"url"`
	Events      []string             `json:"events"`
	CreatedAt   time.Time            `json:"createdAt"`
}

type NewWebhook struct {
	Webhook
	Secret string `json:"secret"`
}

type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "pending"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryFailed    DeliveryStatus = "failed"
)

type deliveryOutcome struct {
	Status         DeliveryStatus
	NextAttemptAt  *time.Time
	LastStatusCode int
	LastError      string
}

type Delivery struct {
	ID             DeliveryID     `json:"id"`
	WebhookID      WebhookID      `json:"webhookId"`
	EventID        EventID        `json:"eventId"`
	EventType      string         `json:"eventType"`
	AttemptCount   int            `json:"attemptCount"`
	Status         DeliveryStatus `json:"status"`
	NextAttemptAt  *time.Time     `json:"nextAttemptAt,omitempty"`
	LastStatusCode int            `json:"lastStatusCode,omitzero"`
	LastError      string         `json:"lastError,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
	DeliveredAt    *time.Time     `json:"deliveredAt,omitempty"`
}

type Service struct {
	db      *sql.DB
	driver  config.DatabaseDriver
	auth    *identity.Service
	secrets secretbox.Keyring
	client  *http.Client
	queue   *jobs.Queue
}

func NewService(
	db *sql.DB,
	driver config.DatabaseDriver,
	auth *identity.Service,
	secrets secretbox.Keyring,
	queue *jobs.Queue,
) *Service {
	return &Service{
		db: db, driver: driver, auth: auth,
		secrets: secrets,
		client:  newHTTPClient(), queue: queue,
	}
}

func (s *Service) SecretKeyConfigured() bool {
	return s.secrets.Configured()
}

func (s *Service) Create(
	ctx context.Context,
	actor identity.Principal,
	workspaceID identity.WorkspaceID,
	input CreateInput,
	audit identity.AuditContext,
) (NewWebhook, error) {
	name := strings.TrimSpace(input.Name)
	urlValue, err := validateURL(input.URL)
	if err != nil || name == "" || len(name) > maxWebhookNameLength {
		return NewWebhook{}, ErrInvalidInput
	}
	events, err := validateEvents(input.Events)
	if !s.secrets.Configured() {
		return NewWebhook{}, ErrSecretKey
	}
	if err != nil {
		return NewWebhook{}, ErrInvalidInput
	}
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.WebhooksManage); err != nil {
		return NewWebhook{}, identity.ErrForbidden
	}
	secret, err := newSecret()
	if err != nil {
		return NewWebhook{}, err
	}
	ciphertext, keyVersion, err := s.secrets.Seal(secret)
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
	var key int64
	if err := s.queryRow(ctx, tx, `
		INSERT INTO webhooks (workspace_id, name, url, secret_ciphertext, secret_key_version, events, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id
	`, workspaceID, name, urlValue, ciphertext, keyVersion, encodeEvents(events), stamp(now)).Scan(&key); err != nil {
		return NewWebhook{}, err
	}
	webhookID := WebhookID(key)
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "webhook.created", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: workspaceID, TargetType: "webhook", TargetKey: int64(webhookID),
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

func (s *Service) List(ctx context.Context, actor identity.Principal, workspaceID identity.WorkspaceID) ([]Webhook, error) {
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
	workspaceID identity.WorkspaceID,
	webhookID WebhookID,
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
		WorkspaceID: workspaceID, TargetType: "webhook", TargetKey: int64(webhookID),
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
	workspaceID identity.WorkspaceID,
	webhookID WebhookID,
	audit identity.AuditContext,
) (string, error) {
	if !s.secrets.Configured() {
		return "", ErrSecretKey
	}
	if _, err := s.auth.AuthorizePrincipal(ctx, actor, workspaceID, identity.WebhooksManage); err != nil {
		return "", identity.ErrForbidden
	}
	secret, err := newSecret()
	if err != nil {
		return "", err
	}
	ciphertext, keyVersion, err := s.secrets.Seal(secret)
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
	result, err := s.exec(ctx, tx, "UPDATE webhooks SET secret_ciphertext = ?, secret_key_version = ? WHERE id = ? AND workspace_id = ?", ciphertext, keyVersion, webhookID, workspaceID)
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
		WorkspaceID: workspaceID, TargetType: "webhook", TargetKey: int64(webhookID),
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
	workspaceID identity.WorkspaceID,
	webhookID WebhookID,
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
		var deliveryKey, webhookKey, eventKey int64
		var nextAttempt, delivered sql.NullString
		var lastError sql.NullString
		var lastStatusCode sql.NullInt64
		var created string
		if err := rows.Scan(&deliveryKey, &webhookKey, &eventKey, &delivery.EventType,
			&delivery.AttemptCount, &delivery.Status, &nextAttempt, &lastStatusCode,
			&lastError, &created, &delivered); err != nil {
			return nil, err
		}
		delivery.ID, delivery.WebhookID, delivery.EventID = DeliveryID(deliveryKey), WebhookID(webhookKey), EventID(eventKey)
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
	workspaceID identity.WorkspaceID,
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
	type target struct {
		id     int64
		events string
	}
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
	if len(targets) == 0 {
		return nil
	}
	var eventKey int64
	if err := s.queryRow(ctx, tx, `
		INSERT INTO webhook_events (workspace_id, event_type, created_at)
		VALUES (?, ?, ?) RETURNING id
	`, workspaceID, eventType, stamp(occurredAt)).Scan(&eventKey); err != nil {
		return err
	}
	eventID := EventID(eventKey)
	for _, target := range targets {
		envelope, err := json.Marshal(map[string]any{
			"id": eventID, "type": eventType, "timestamp": occurredAt.UTC(),
			"workspaceId": workspaceID, "data": raw,
		})
		if err != nil {
			return err
		}
		var deliveryKey int64
		err = s.queryRow(ctx, tx, `
			INSERT INTO webhook_deliveries (
				webhook_id, event_id, event_type, payload, attempt_count,
				status, next_attempt_at, last_status_code, last_error, created_at
			) VALUES (?, ?, ?, ?, 0, 'pending', ?, NULL, NULL, ?)
			ON CONFLICT (webhook_id, event_id) DO NOTHING
			RETURNING id
		`, target.id, eventKey, eventType, string(envelope), stamp(occurredAt), stamp(occurredAt)).Scan(&deliveryKey)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if s.queue == nil {
			return errors.New("webhook job queue is unavailable")
		}
		jobPayload, err := json.Marshal(map[string]int64{"deliveryId": deliveryKey})
		if err != nil {
			return err
		}
		if err := s.queue.EnqueueTx(ctx, tx, workspaceID, JobKind, deliveryKey, string(jobPayload), occurredAt); err != nil {
			return err
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
	deliveryID, err := jobDeliveryID(job)
	if err != nil {
		return jobs.Failure{Category: "payload-invalid"}
	}
	return s.deliverOne(ctx, job.WorkspaceID, deliveryID, job.AttemptCount)
}

func (s *Service) HandleExhaustedJob(ctx context.Context, job jobs.Job) error {
	deliveryID, err := jobDeliveryID(job)
	if err != nil {
		return nil
	}
	outcome := evaluateDeliveryOutcome(job.AttemptCount, maxAttempts, 0, "worker-exhausted", time.Now().UTC())
	result, err := s.exec(ctx, s.db, `
		UPDATE webhook_deliveries SET status = ?, attempt_count = CASE
			WHEN attempt_count < ? THEN ? ELSE attempt_count END,
			next_attempt_at = NULL, last_error = ?
		WHERE id = ? AND status = 'pending'
			AND webhook_id IN (SELECT id FROM webhooks WHERE workspace_id = ?)
	`, string(outcome.Status), maxAttempts, maxAttempts, outcome.LastError, deliveryID, job.WorkspaceID)
	if err != nil {
		return jobs.Failure{Category: "storage-error"}
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return jobs.Failure{Category: "storage-error"}
	}
	if changed == 1 {
		telemetry.RecordWebhookDelivery(ctx, string(outcome.Status))
	}
	return nil
}

func jobDeliveryID(job jobs.Job) (int64, error) {
	var payload struct {
		DeliveryID int64 `json:"deliveryId"`
	}
	if err := json.Unmarshal([]byte(job.Payload), &payload); err != nil || payload.DeliveryID <= 0 {
		return 0, errors.New("job payload is invalid")
	}
	return payload.DeliveryID, nil
}

func (s *Service) deliverOne(ctx context.Context, workspaceID identity.WorkspaceID, deliveryID int64, jobAttempt int) error {
	now := time.Now().UTC()
	var webhookID, eventKey int64
	var eventType, payload, urlValue, encrypted string
	var attempt, keyVersion int
	err := s.queryRow(ctx, s.db, `
		SELECT d.id, d.webhook_id, d.event_id, d.event_type, d.payload,
			d.attempt_count, w.url, w.secret_ciphertext, w.secret_key_version
		FROM webhook_deliveries d JOIN webhooks w ON w.id = d.webhook_id
		WHERE d.id = ? AND d.status = 'pending' AND w.workspace_id = ?
	`, deliveryID, workspaceID).Scan(&deliveryID, &webhookID, &eventKey, &eventType, &payload, &attempt, &urlValue, &encrypted, &keyVersion)
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
	secret, err := s.secrets.Open(keyVersion, encrypted)
	if err != nil {
		return s.deliveryFailure(ctx, deliveryID, attempt, 0, "secret-unavailable", now)
	}
	eventID := EventID(eventKey).String()
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
	outcome := evaluateDeliveryOutcome(attempt, maxAttempts, response.StatusCode, "", now)
	if _, err := s.exec(ctx, s.db, `
		UPDATE webhook_deliveries SET status = ?, delivered_at = ?,
		next_attempt_at = NULL, last_status_code = ?, last_error = NULL WHERE id = ? AND status = 'pending'
	`, string(outcome.Status), stamp(now), outcome.LastStatusCode, deliveryID); err != nil {
		return jobs.Failure{Category: "storage-error"}
	}
	telemetry.RecordWebhookDelivery(ctx, string(outcome.Status))
	return nil
}

func (s *Service) deliveryFailure(
	ctx context.Context,
	deliveryID int64,
	attempt, statusCode int,
	reason string,
	now time.Time,
) error {
	outcome := evaluateDeliveryOutcome(attempt, maxAttempts, statusCode, reason, now)
	var next any
	if outcome.NextAttemptAt != nil {
		next = stamp(*outcome.NextAttemptAt)
	}
	if _, err := s.exec(ctx, s.db, `
		UPDATE webhook_deliveries SET status = ?, next_attempt_at = ?,
		last_status_code = ?, last_error = ? WHERE id = ? AND status = 'pending'
	`, string(outcome.Status), next, nullableStatus(outcome.LastStatusCode), outcome.LastError, deliveryID); err != nil {
		return jobs.Failure{Category: "storage-error"}
	}
	telemetry.RecordWebhookDelivery(ctx, string(outcome.Status))
	return jobs.Failure{Category: reason}
}

func evaluateDeliveryOutcome(attempt, maxAttempts, statusCode int, reason string, now time.Time) deliveryOutcome {
	if attempt < 1 {
		attempt = 1
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	if reason == "" && statusCode >= 200 && statusCode < 300 {
		return deliveryOutcome{
			Status:         DeliveryDelivered,
			NextAttemptAt:  nil,
			LastStatusCode: statusCode,
			LastError:      "",
		}
	}
	if attempt >= maxAttempts {
		return deliveryOutcome{
			Status:         DeliveryFailed,
			NextAttemptAt:  nil,
			LastStatusCode: statusCode,
			LastError:      reason,
		}
	}
	next := now.Add(jobs.RetryDelay(attempt))
	return deliveryOutcome{
		Status:         DeliveryPending,
		NextAttemptAt:  &next,
		LastStatusCode: statusCode,
		LastError:      reason,
	}
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
	var key int64
	var events, created string
	if err := row.Scan(&key, &webhook.WorkspaceID, &webhook.Name, &webhook.URL, &events, &created); err != nil {
		return Webhook{}, err
	}
	webhook.ID = WebhookID(key)
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
