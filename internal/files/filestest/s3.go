// Package filestest provides an in-process S3-compatible endpoint for tests of
// the files package and the REST handlers that depend on it.
package filestest

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const amzDateLayout = "20060102T150405Z"

// S3Endpoint starts a path-style S3-compatible endpoint and points the AWS SDK
// credential chain at fixed test credentials. It returns the endpoint URL, which
// the caller passes as the storage endpoint. The endpoint does not verify SigV4
// signatures. It refuses unsigned requests, enforces presigned expiry, honors
// If-None-Match: *, and decodes aws-chunked uploads.
func S3Endpoint(t testing.TB) string {
	t.Helper()
	t.Setenv("AWS_ACCESS_KEY_ID", "conformance-access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "conformance-secret")
	t.Setenv("AWS_SESSION_TOKEN", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(t.TempDir(), "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(t.TempDir(), "credentials"))
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	server := httptest.NewServer(&fakeS3{objects: map[string][]byte{}, types: map[string]string{}})
	t.Cleanup(server.Close)
	return server.URL
}

// fakeS3 is a minimal path-style S3 endpoint.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
	types   map[string]string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isFakeS3Signed(r) {
		writeFakeS3Error(w, http.StatusForbidden, "AccessDenied", "request is not signed")
		return
	}
	if fakeS3PresignExpired(r, time.Now()) {
		writeFakeS3Error(w, http.StatusForbidden, "AccessDenied", "Request has expired")
		return
	}
	switch r.Method {
	case http.MethodPut:
		f.put(w, r)
	case http.MethodGet, http.MethodHead:
		f.get(w, r)
	default:
		writeFakeS3Error(w, http.StatusMethodNotAllowed, "MethodNotAllowed", "method not allowed")
	}
}

func (f *fakeS3) put(w http.ResponseWriter, r *http.Request) {
	var body []byte
	var err error
	if strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") {
		body, err = readFakeS3ChunkedBody(r.Body)
	} else {
		body, err = io.ReadAll(r.Body)
	}
	if err != nil {
		writeFakeS3Error(w, http.StatusBadRequest, "IncompleteBody", "body could not be decoded")
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.objects[r.URL.Path]; exists && r.Header.Get("If-None-Match") == "*" {
		writeFakeS3Error(w, http.StatusPreconditionFailed, "PreconditionFailed", "object exists")
		return
	}
	f.objects[r.URL.Path] = body
	f.types[r.URL.Path] = r.Header.Get("Content-Type")
	w.WriteHeader(http.StatusOK)
}

func (f *fakeS3) get(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, exists := f.objects[r.URL.Path]
	if !exists {
		writeFakeS3Error(w, http.StatusNotFound, "NoSuchKey", "The specified key does not exist.")
		return
	}
	w.Header().Set("Content-Type", f.types[r.URL.Path])
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(body)
	}
}

func isFakeS3Signed(r *http.Request) bool {
	return r.URL.Query().Get("X-Amz-Signature") != "" || strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 ")
}

func fakeS3PresignExpired(r *http.Request, now time.Time) bool {
	query := r.URL.Query()
	if query.Get("X-Amz-Signature") == "" {
		return false
	}
	issued, err := time.Parse(amzDateLayout, query.Get("X-Amz-Date"))
	if err != nil {
		return true
	}
	expires, err := strconv.Atoi(query.Get("X-Amz-Expires"))
	if err != nil {
		return true
	}
	return now.After(issued.Add(time.Duration(expires) * time.Second))
}

// readFakeS3ChunkedBody decodes an aws-chunked payload and ignores trailers.
func readFakeS3ChunkedBody(source io.Reader) ([]byte, error) {
	reader := bufio.NewReader(source)
	var out bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		sizeText, _, _ := strings.Cut(strings.TrimSpace(line), ";")
		size, err := strconv.ParseInt(sizeText, 16, 64)
		if err != nil {
			return nil, err
		}
		if size == 0 {
			return out.Bytes(), nil
		}
		chunk := make([]byte, size)
		if _, err := io.ReadFull(reader, chunk); err != nil {
			return nil, err
		}
		out.Write(chunk)
		if _, err := reader.Discard(2); err != nil {
			return nil, err
		}
	}
}

func writeFakeS3Error(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message></Error>`, code, message)
}
