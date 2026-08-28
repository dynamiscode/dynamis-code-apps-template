package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
	"example.com/dynamis-code/apps-template/internal/platform/telemetry"
)

const (
	maxPayloadBytes = 1 << 20
	maxKindLength   = 100
	maxKeyLength    = 255
	maxAttempts     = 5
	leaseDuration   = time.Minute
	initialRetry    = time.Second
)

type Job struct {
	ID           string
	WorkspaceID  string
	Kind         string
	Payload      string
	AttemptCount int
	AvailableAt  time.Time
	LeaseToken   string
	StartedAt    *time.Time
	CreatedAt    time.Time
}

type Handler func(context.Context, Job) error

type Failure struct{ Category string }

func (failure Failure) Error() string { return failure.Category }

type Queue struct {
	db        *sql.DB
	driver    config.DatabaseDriver
	logger    *slog.Logger
	handlers  map[string]Handler
	wake      chan struct{}
	process   sync.Mutex
	stateMu   sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
}

func NewQueue(db *sql.DB, driver config.DatabaseDriver, logger *slog.Logger) *Queue {
	if logger == nil {
		logger = slog.Default()
	}
	return &Queue{
		db: db, driver: driver, logger: logger,
		handlers: make(map[string]Handler), wake: make(chan struct{}, 1),
	}
}

func (queue *Queue) Register(kind string, handler Handler) error {
	if strings.TrimSpace(kind) == "" || handler == nil {
		return errors.New("job handler is invalid")
	}
	queue.stateMu.Lock()
	defer queue.stateMu.Unlock()
	if queue.cancel != nil {
		return errors.New("job queue already started")
	}
	if _, exists := queue.handlers[kind]; exists {
		return errors.New("job handler already registered")
	}
	queue.handlers[kind] = handler
	return nil
}

func (queue *Queue) EnqueueTx(
	ctx context.Context,
	tx *sql.Tx,
	workspaceID, kind, deduplicationKey, payload string,
	availableAt time.Time,
) error {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(kind) == "" ||
		len(kind) > maxKindLength || strings.TrimSpace(deduplicationKey) == "" ||
		len(deduplicationKey) > maxKeyLength || len(payload) == 0 ||
		len(payload) > maxPayloadBytes || !json.Valid([]byte(payload)) {
		return errors.New("job input is invalid")
	}
	jobID, err := id.New()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, database.Rebind(queue.driver, `
		INSERT INTO background_jobs (
			id, workspace_id, kind, deduplication_key, payload, status,
			attempt_count, available_at, created_at
		) VALUES (?, ?, ?, ?, ?, 'pending', 0, ?, ?)
		ON CONFLICT (workspace_id, kind, deduplication_key) DO NOTHING
	`), jobID, workspaceID, kind, deduplicationKey, payload,
		stamp(availableAt), stamp(availableAt))
	if err == nil {
		select {
		case queue.wake <- struct{}{}:
		default:
		}
	}
	return err
}

func (queue *Queue) Start(parent context.Context) {
	queue.stateMu.Lock()
	defer queue.stateMu.Unlock()
	if queue.cancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(parent)
	queue.cancel, queue.done = cancel, make(chan struct{})
	go queue.run(ctx)
}

func (queue *Queue) Close() {
	queue.closeOnce.Do(func() {
		queue.stateMu.Lock()
		cancel, done := queue.cancel, queue.done
		queue.stateMu.Unlock()
		if cancel != nil {
			cancel()
			<-done
		}
	})
}

func (queue *Queue) Process(ctx context.Context, limit int) (int, error) {
	if limit < 1 {
		return 0, errors.New("job limit is invalid")
	}
	queue.process.Lock()
	defer queue.process.Unlock()
	processed := 0
	for processed < limit {
		job, ok, err := queue.claim(ctx)
		if err != nil || !ok {
			return processed, err
		}
		handler, exists := queue.handler(job.Kind)
		if !exists {
			if err := queue.finish(ctx, job, Failure{Category: "handler-unavailable"}); err != nil {
				return processed, err
			}
			processed++
			continue
		}
		var handlerErr error
		if job.AttemptCount <= maxAttempts {
			handlerErr = handler(ctx, job)
		} else {
			handlerErr = Failure{Category: "lease-expired"}
		}
		if err := queue.finish(ctx, job, handlerErr); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (queue *Queue) run(ctx context.Context) {
	defer close(queue.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		_, _ = queue.Process(ctx, 1)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-queue.wake:
		}
	}
}

func (queue *Queue) handler(kind string) (Handler, bool) {
	queue.stateMu.Lock()
	defer queue.stateMu.Unlock()
	handler, exists := queue.handlers[kind]
	return handler, exists
}

func (queue *Queue) claim(ctx context.Context) (Job, bool, error) {
	now := time.Now().UTC()
	leaseToken, err := id.New()
	if err != nil {
		return Job{}, false, err
	}
	tx, err := queue.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return Job{}, false, err
	}
	defer tx.Rollback()
	query := `
		UPDATE background_jobs SET status = 'running', attempt_count = attempt_count + 1,
			lease_token = ?, leased_until = ?, started_at = ?,
			completed_at = NULL
		WHERE id = (
			SELECT id FROM background_jobs
			WHERE (status = 'pending' AND available_at <= ?)
				OR (status = 'running' AND leased_until IS NOT NULL AND leased_until <= ?)
			ORDER BY available_at, created_at, id LIMIT 1
		)
		AND (status = 'pending' OR (status = 'running' AND leased_until IS NOT NULL AND leased_until <= ?))
	`
	result, err := tx.ExecContext(ctx, database.Rebind(queue.driver, query),
		leaseToken, stamp(now.Add(leaseDuration)), stamp(now),
		stamp(now), stamp(now), stamp(now))
	if err != nil {
		return Job{}, false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return Job{}, false, err
	}
	if changed != 1 {
		return Job{}, false, nil
	}
	var job Job
	var available, started, created string
	err = tx.QueryRowContext(ctx, database.Rebind(queue.driver, `
		SELECT id, workspace_id, kind, payload, attempt_count, available_at,
			started_at, created_at FROM background_jobs WHERE lease_token = ?
	`), leaseToken).Scan(&job.ID, &job.WorkspaceID, &job.Kind, &job.Payload,
		&job.AttemptCount, &available, &started, &created)
	if err != nil {
		return Job{}, false, err
	}
	job.LeaseToken = leaseToken
	if job.AvailableAt, err = time.Parse(time.RFC3339Nano, available); err != nil {
		return Job{}, false, err
	}
	if started != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, started)
		if parseErr != nil {
			return Job{}, false, parseErr
		}
		job.StartedAt = &parsed
	}
	if job.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Job{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

func (queue *Queue) finish(ctx context.Context, job Job, handlerErr error) error {
	now := time.Now().UTC()
	status, lastError, next := "succeeded", any(nil), any(nil)
	if handlerErr != nil {
		status = "pending"
		lastError = failureCategory(handlerErr)
		next = stamp(now.Add(retryDelay(job.AttemptCount)))
		if job.AttemptCount >= maxAttempts {
			status, next = "failed", nil
		}
	}
	_, err := queue.db.ExecContext(ctx, database.Rebind(queue.driver, `
		UPDATE background_jobs SET status = ?, available_at = COALESCE(?, available_at),
			lease_token = NULL, leased_until = NULL,
			completed_at = CASE WHEN ? IN ('succeeded', 'failed') THEN ? ELSE NULL END,
			last_error = ? WHERE id = ? AND status = 'running' AND lease_token = ?
	`), status, next, status, stamp(now), lastError, job.ID, job.LeaseToken)
	if err != nil {
		return err
	}
	telemetry.RecordJob(ctx, job.Kind, status)
	if handlerErr != nil {
		queue.logger.Warn("background job failed", "kind", job.Kind, "attempt", job.AttemptCount, "status", status, "category", lastError)
	} else {
		queue.logger.Info("background job completed", "kind", job.Kind, "attempt", job.AttemptCount)
	}
	return nil
}

func failureCategory(err error) string {
	var failure Failure
	if errors.As(err, &failure) && validCategory(failure.Category) {
		return failure.Category
	}
	return "handler-error"
}

func validCategory(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if (character < 'a' || character > 'z') && character != '-' {
			return false
		}
	}
	return true
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	return initialRetry * time.Duration(1<<(attempt-1))
}

func stamp(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
