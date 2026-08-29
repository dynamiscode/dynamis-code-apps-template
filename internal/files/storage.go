package files

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

var (
	ErrObjectNotFound = errors.New("file object not found")
	ErrSizeMismatch   = errors.New("file object size mismatch")
	ErrNotSupported   = errors.New("operation is not supported by this store")
)

type ObjectStore interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Get(context.Context, string) (io.ReadCloser, error)
	Head(context.Context, string) (ObjectInfo, error)
	SupportsPresignedPut() bool
	PresignPut(context.Context, string, int64, string, time.Duration) (PresignedUpload, error)
	PresignGet(context.Context, string, time.Duration) (string, error)
}

type PresignedUpload struct {
	URL     string
	Headers map[string]string
}

type ObjectInfo struct {
	Size        int64
	ContentType string
}

func NewStore(ctx context.Context, cfg config.Storage) (ObjectStore, error) {
	if cfg.Driver == config.StorageS3 {
		return newS3Store(ctx, cfg)
	}
	return newLocalStore(cfg.LocalPath)
}

type localStore struct{ root string }

func newLocalStore(root string) (*localStore, error) {
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	return &localStore{root: root}, nil
}

func (s *localStore) Put(_ context.Context, key string, source io.Reader, size int64, _ string) error {
	target, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	count, copyErr := io.Copy(temporary, io.LimitReader(source, size+1))
	if copyErr != nil {
		temporary.Close()
		return copyErr
	}
	if count != size {
		temporary.Close()
		return ErrSizeMismatch
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, target)
}

func (s *localStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrObjectNotFound
	}
	return file, err
}

func (s *localStore) Head(_ context.Context, key string) (ObjectInfo, error) {
	path, err := s.path(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return ObjectInfo{}, ErrObjectNotFound
	}
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{Size: info.Size()}, nil
}

func (*localStore) PresignPut(context.Context, string, int64, string, time.Duration) (PresignedUpload, error) {
	return PresignedUpload{}, ErrNotSupported
}

func (*localStore) SupportsPresignedPut() bool { return false }

func (*localStore) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", ErrNotSupported
}

func (s *localStore) path(key string) (string, error) {
	if key == "" || strings.ContainsAny(key, "\\\x00") {
		return "", errors.New("invalid object key")
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.New("invalid object key")
	}
	return filepath.Join(s.root, clean), nil
}
