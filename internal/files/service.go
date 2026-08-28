package files

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/identity"
	"example.com/dynamis-code/apps-template/internal/platform/config"
	"example.com/dynamis-code/apps-template/internal/platform/database"
	"example.com/dynamis-code/apps-template/internal/platform/id"
)

const (
	Pending = "pending"
	Ready   = "ready"
	Failed  = "failed"
)

var (
	ErrInvalidInput = errors.New("file input is invalid")
	ErrNotFound     = errors.New("file not found")
	ErrLimit        = errors.New("file storage limit reached")
	ErrNotReady     = errors.New("file is not ready")
)

type File struct {
	ID           string    `json:"id"`
	WorkspaceID  string    `json:"workspaceId"`
	OwnerUserID  *string   `json:"ownerUserId"`
	OriginalName string    `json:"originalName"`
	DetectedMIME *string   `json:"detectedMime,omitempty"`
	Size         int64     `json:"size"`
	SHA256       *string   `json:"sha256,omitempty"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type Service struct {
	db                *sql.DB
	driver            config.DatabaseDriver
	auth              *identity.Service
	store             ObjectStore
	maxObjectBytes    int64
	maxWorkspaceBytes int64
	signedURLTTL      time.Duration
	prefix            string
	now               func() time.Time
}

type InitiateInput struct {
	OriginalName string
	Size         int64
	ContentType  string
}

func NewService(db *sql.DB, driver config.DatabaseDriver, auth *identity.Service, store ObjectStore, maxObjectBytes, maxWorkspaceBytes int64, signedURLTTL time.Duration, prefix string) *Service {
	return &Service{db: db, driver: driver, auth: auth, store: store, maxObjectBytes: maxObjectBytes, maxWorkspaceBytes: maxWorkspaceBytes, signedURLTTL: signedURLTTL, prefix: prefix, now: time.Now}
}

func (s *Service) Initiate(ctx context.Context, actor identity.Principal, workspaceID string, input InitiateInput, audit identity.AuditContext) (File, error) {
	name, err := safeFilename(input.OriginalName)
	if err != nil || input.Size < 1 || input.Size > s.maxObjectBytes || !allowedDeclaredType(input.ContentType, name) {
		return File{}, ErrInvalidInput
	}
	file, err := s.reserve(ctx, actor, workspaceID, name, input.Size, audit)
	if err != nil {
		return File{}, err
	}
	return file, nil
}

func (s *Service) Upload(ctx context.Context, actor identity.Principal, workspaceID, originalName string, source io.Reader, audit identity.AuditContext) (File, error) {
	name, err := safeFilename(originalName)
	if err != nil {
		return File{}, ErrInvalidInput
	}
	temporary, size, detected, digest, err := inspect(source, s.maxObjectBytes)
	if err != nil {
		return File{}, err
	}
	defer os.Remove(temporary)
	if !validContent(name, detected) {
		return File{}, ErrInvalidInput
	}
	file, err := s.reserve(ctx, actor, workspaceID, name, size, audit)
	if err != nil {
		return File{}, err
	}
	input, err := os.Open(temporary)
	if err != nil {
		return File{}, err
	}
	putErr := s.store.Put(ctx, s.objectKey(file), input, size, detected)
	closeErr := input.Close()
	if putErr != nil {
		s.markFailed(ctx, file.ID)
		return File{}, putErr
	}
	if closeErr != nil {
		return File{}, closeErr
	}
	return s.complete(ctx, actor, file, size, detected, digest, audit)
}

func (s *Service) PutContent(ctx context.Context, actor identity.Principal, workspaceID, fileID string, source io.Reader, audit identity.AuditContext) (File, error) {
	file, err := s.pending(ctx, actor, workspaceID, fileID)
	if err != nil {
		return File{}, err
	}
	temporary, size, detected, digest, err := inspect(source, s.maxObjectBytes)
	if err != nil || size != file.Size || !validContent(file.OriginalName, detected) {
		s.markFailed(ctx, file.ID)
		if err != nil {
			return File{}, err
		}
		return File{}, ErrInvalidInput
	}
	defer os.Remove(temporary)
	input, err := os.Open(temporary)
	if err != nil {
		return File{}, err
	}
	putErr := s.store.Put(ctx, s.objectKey(file), input, size, detected)
	closeErr := input.Close()
	if putErr != nil {
		s.markFailed(ctx, file.ID)
		return File{}, putErr
	}
	if closeErr != nil {
		return File{}, closeErr
	}
	return s.complete(ctx, actor, file, size, detected, digest, audit)
}

func (s *Service) Complete(ctx context.Context, actor identity.Principal, workspaceID, fileID string, audit identity.AuditContext) (File, error) {
	if !validID(workspaceID) || !validID(fileID) {
		return File{}, ErrInvalidInput
	}
	file, err := s.pending(ctx, actor, workspaceID, fileID)
	if err != nil {
		return File{}, err
	}
	if file.Status != Pending {
		return File{}, ErrNotReady
	}
	object, err := s.store.Head(ctx, s.objectKey(file))
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return File{}, ErrNotFound
		}
		return File{}, err
	}
	if object.Size < 1 || object.Size > s.maxObjectBytes {
		return File{}, ErrInvalidInput
	}
	reader, err := s.store.Get(ctx, s.objectKey(file))
	if err != nil {
		if errors.Is(err, ErrObjectNotFound) {
			return File{}, ErrNotFound
		}
		return File{}, err
	}
	temporary, size, detected, digest, inspectErr := inspect(reader, s.maxObjectBytes)
	reader.Close()
	if inspectErr != nil || size != object.Size || !validContent(file.OriginalName, detected) {
		s.markFailed(ctx, file.ID)
		return File{}, ErrInvalidInput
	}
	os.Remove(temporary)
	return s.complete(ctx, actor, file, size, detected, digest, audit)
}

func (s *Service) List(ctx context.Context, actor identity.Principal, workspaceID string, limit int) ([]File, error) {
	if limit < 1 || limit > 100 {
		return nil, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesRead); err != nil {
		return nil, identity.ErrForbidden
	}
	rows, err := tx.QueryContext(ctx, database.Rebind(s.driver, `
		SELECT id, workspace_id, owner_user_id, original_name, detected_mime,
			size, sha256, status, created_at, updated_at
		FROM files WHERE workspace_id = ? AND status = ?
		ORDER BY created_at DESC, id DESC LIMIT ?
	`), workspaceID, Ready, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]File, 0)
	for rows.Next() {
		file, err := scan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, file)
	}
	return result, rows.Err()
}

func (s *Service) Get(ctx context.Context, actor identity.Principal, workspaceID, fileID string) (File, error) {
	return s.get(ctx, actor, workspaceID, fileID, identity.ResourcesRead)
}

func (s *Service) pending(ctx context.Context, actor identity.Principal, workspaceID, fileID string) (File, error) {
	if !validID(workspaceID) || !validID(fileID) {
		return File{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return File{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return File{}, identity.ErrForbidden
	}
	var file File
	err = scanRow(tx.QueryRowContext(ctx, database.Rebind(s.driver, `
		SELECT id, workspace_id, owner_user_id, original_name, detected_mime,
			size, sha256, status, created_at, updated_at
		FROM files WHERE id = ? AND workspace_id = ? AND status = ?
	`), fileID, workspaceID, Pending), &file)
	if errors.Is(err, sql.ErrNoRows) {
		return File{}, ErrNotFound
	}
	return file, err
}

func (s *Service) Open(ctx context.Context, actor identity.Principal, workspaceID, fileID string) (File, io.ReadCloser, error) {
	file, err := s.Get(ctx, actor, workspaceID, fileID)
	if err != nil {
		return File{}, nil, err
	}
	reader, err := s.store.Get(ctx, s.objectKey(file))
	if errors.Is(err, ErrObjectNotFound) {
		return File{}, nil, ErrNotFound
	}
	if err != nil {
		return File{}, nil, err
	}
	return file, reader, nil
}

func (s *Service) PresignedGet(ctx context.Context, actor identity.Principal, workspaceID, fileID string) (File, string, error) {
	file, err := s.Get(ctx, actor, workspaceID, fileID)
	if err != nil {
		return File{}, "", err
	}
	url, err := s.store.PresignGet(ctx, s.objectKey(file), s.signedURLTTL)
	return file, url, err
}

func (s *Service) PresignedPut(ctx context.Context, file File) (string, error) {
	return s.store.PresignPut(ctx, s.objectKey(file), file.Size, contentType(file), s.signedURLTTL)
}

func (s *Service) reserve(ctx context.Context, actor identity.Principal, workspaceID, name string, size int64, audit identity.AuditContext) (File, error) {
	fileID, err := id.New()
	if err != nil {
		return File{}, err
	}
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return File{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, identity.ResourcesWrite); err != nil {
		return File{}, identity.ErrForbidden
	}
	var used int64
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver,
		"SELECT COALESCE(SUM(size), 0) FROM files WHERE workspace_id = ? AND status IN (?, ?)",
	), workspaceID, Pending, Ready).Scan(&used); err != nil {
		return File{}, err
	}
	if used > s.maxWorkspaceBytes-size {
		return File{}, ErrLimit
	}
	file := File{ID: fileID, WorkspaceID: workspaceID, OwnerUserID: stringPtr(actor.UserID), OriginalName: name, Size: size, Status: Pending, CreatedAt: now, UpdatedAt: now}
	if _, err := tx.ExecContext(ctx, database.Rebind(s.driver, `
		INSERT INTO files (id, workspace_id, owner_user_id, object_key, original_name, size, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), file.ID, workspaceID, actor.UserID, s.objectKey(file), name, size, Pending, stamp(now), stamp(now)); err != nil {
		return File{}, err
	}
	if err := tx.Commit(); err != nil {
		return File{}, err
	}
	return file, nil
}

func (s *Service) complete(ctx context.Context, actor identity.Principal, file File, size int64, detected string, digest [32]byte, audit identity.AuditContext) (File, error) {
	now := s.now().UTC()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return File{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, file.WorkspaceID, identity.ResourcesWrite); err != nil {
		return File{}, identity.ErrForbidden
	}
	var used int64
	if err := tx.QueryRowContext(ctx, database.Rebind(s.driver,
		"SELECT COALESCE(SUM(size), 0) FROM files WHERE workspace_id = ? AND id <> ? AND status IN (?, ?)",
	), file.WorkspaceID, file.ID, Pending, Ready).Scan(&used); err != nil {
		return File{}, err
	}
	if used > s.maxWorkspaceBytes-size {
		return File{}, ErrLimit
	}
	sha := hex.EncodeToString(digest[:])
	_, err = tx.ExecContext(ctx, database.Rebind(s.driver, `
		UPDATE files SET detected_mime = ?, size = ?, sha256 = ?, status = ?, updated_at = ?
		WHERE id = ? AND workspace_id = ? AND status = ?
	`), detected, size, sha, Ready, stamp(now), file.ID, file.WorkspaceID, Pending)
	if err != nil {
		return File{}, err
	}
	if err := s.auth.RecordAuditInTx(ctx, tx, identity.AuditEvent{
		EventType: "file.uploaded", ActorUserID: actor.UserID, AuthMethod: actor.AuthMethod,
		WorkspaceID: file.WorkspaceID, TargetType: "file", TargetID: file.ID,
		Action: "file.upload", Outcome: "success", RequestID: audit.RequestID,
		SourceAddress: audit.SourceAddress, Metadata: "{}", CreatedAt: now,
	}); err != nil {
		return File{}, err
	}
	if err := tx.Commit(); err != nil {
		return File{}, err
	}
	file.DetectedMIME = stringPtr(detected)
	file.SHA256 = stringPtr(sha)
	file.Size, file.Status, file.UpdatedAt = size, Ready, now
	return file, nil
}

func (s *Service) get(ctx context.Context, actor identity.Principal, workspaceID, fileID string, permission identity.Permission) (File, error) {
	if !validID(workspaceID) || !validID(fileID) {
		return File{}, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return File{}, err
	}
	defer tx.Rollback()
	if _, err := s.auth.AuthorizePrincipalInTx(ctx, tx, actor, workspaceID, permission); err != nil {
		return File{}, identity.ErrForbidden
	}
	var file File
	row := tx.QueryRowContext(ctx, database.Rebind(s.driver, `
		SELECT id, workspace_id, owner_user_id, original_name, detected_mime,
			size, sha256, status, created_at, updated_at
		FROM files WHERE id = ? AND workspace_id = ? AND status = ?
	`), fileID, workspaceID, Ready)
	if err := scanRow(row, &file); errors.Is(err, sql.ErrNoRows) {
		return File{}, ErrNotFound
	} else if err != nil {
		return File{}, err
	}
	return file, nil
}

func (s *Service) markFailed(ctx context.Context, fileID string) {
	_, _ = s.db.ExecContext(ctx, database.Rebind(s.driver,
		"UPDATE files SET status = ?, updated_at = ? WHERE id = ? AND status = ?",
	), Failed, stamp(s.now()), fileID, Pending)
}

func scan(rows *sql.Rows) (File, error) {
	var file File
	err := scanRow(rows, &file)
	return file, err
}

type scanner interface {
	Scan(...any) error
}

func scanRow(row scanner, file *File) error {
	var owner, detected, digest sql.NullString
	var createdAt, updatedAt string
	if err := row.Scan(&file.ID, &file.WorkspaceID, &owner, &file.OriginalName, &detected, &file.Size, &digest, &file.Status, &createdAt, &updatedAt); err != nil {
		return err
	}
	var err error
	file.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return err
	}
	file.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return err
	}
	if owner.Valid {
		file.OwnerUserID = &owner.String
	}
	if detected.Valid {
		file.DetectedMIME = &detected.String
	}
	if digest.Valid {
		file.SHA256 = &digest.String
	}
	return nil
}

func inspect(source io.Reader, maximum int64) (string, int64, string, [32]byte, error) {
	temporary, err := os.CreateTemp("", "dynamis-file-*")
	if err != nil {
		return "", 0, "", [32]byte{}, err
	}
	name := temporary.Name()
	defer func() { temporary.Close() }()
	hash := sha256.New()
	prefix := &prefixWriter{}
	count, err := io.Copy(temporary, io.LimitReader(io.TeeReader(io.TeeReader(source, hash), prefix), maximum+1))
	if err != nil {
		os.Remove(name)
		return "", 0, "", [32]byte{}, err
	}
	if count < 1 || count > maximum {
		os.Remove(name)
		return "", 0, "", [32]byte{}, ErrLimit
	}
	if err := temporary.Close(); err != nil {
		os.Remove(name)
		return "", 0, "", [32]byte{}, err
	}
	detected, _, err := mime.ParseMediaType(http.DetectContentType(prefix.bytes))
	if err != nil {
		os.Remove(name)
		return "", 0, "", [32]byte{}, ErrInvalidInput
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	return name, count, detected, digest, nil
}

type prefixWriter struct{ bytes []byte }

func (w *prefixWriter) Write(value []byte) (int, error) {
	length := len(value)
	if len(w.bytes) < 512 {
		remaining := 512 - len(w.bytes)
		if len(value) > remaining {
			value = value[:remaining]
		}
		w.bytes = append(w.bytes, value...)
	}
	return length, nil
}

func validContent(name, detected string) bool {
	expected := mimeByExtension(filepath.Ext(name))
	if expected == "" || !allowedMIME(detected) {
		return false
	}
	return expected == detected
}

func allowedDeclaredType(value, name string) bool {
	if strings.TrimSpace(value) == "" {
		return true
	}
	detected, _, err := mime.ParseMediaType(value)
	if err != nil || !allowedMIME(detected) {
		return false
	}
	return mimeByExtension(filepath.Ext(name)) == detected || detected == "application/octet-stream"
}

func allowedMIME(value string) bool {
	switch value {
	case "text/plain", "application/pdf", "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

func mimeByExtension(extension string) string {
	switch strings.ToLower(extension) {
	case ".txt", ".csv":
		return "text/plain"
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return ""
	}
}

func safeFilename(value string) (string, error) {
	value = strings.ReplaceAll(value, "\\", "/")
	name := filepath.Base(value)
	if name == "." || name == ".." || name == "" || len(name) > 255 || strings.TrimSpace(name) == "" || strings.ContainsAny(name, "/\x00\r\n") {
		return "", ErrInvalidInput
	}
	for _, character := range name {
		if character < 0x20 || character == 0x7f {
			return "", ErrInvalidInput
		}
	}
	return name, nil
}

func (s *Service) objectKey(file File) string {
	if s.prefix == "" {
		return file.WorkspaceID + "/" + file.ID
	}
	return s.prefix + "/" + file.WorkspaceID + "/" + file.ID
}

func contentType(file File) string {
	if file.DetectedMIME != nil {
		return *file.DetectedMIME
	}
	return mimeByExtension(filepath.Ext(file.OriginalName))
}

func stringPtr(value string) *string { return &value }

func stamp(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func validID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
