package files

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"

	"example.com/dynamis-code/apps-template/internal/files/filestest"
	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// TestS3ConformanceInProcess runs the S3 object contract against an in-process
// S3-compatible endpoint through the production constructor. The endpoint does
// not verify SigV4 signatures. It refuses unsigned requests, enforces presigned
// expiry, honors If-None-Match: *, and decodes aws-chunked uploads, so it checks
// the wire behavior the adapter depends on without network access.
func TestS3ConformanceInProcess(t *testing.T) {
	store, err := newS3Store(context.Background(), config.Storage{
		Driver: config.StorageS3, S3Endpoint: filestest.S3Endpoint(t), S3Region: "us-east-1",
		S3Bucket: "files", S3ForcePathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	runS3Conformance(t, store, "conformance-"+randomS3Suffix(t))
}

// TestS3ConformanceConfiguredEndpoint runs the same contract against a real
// S3-compatible endpoint such as R2, MinIO, or AWS S3. It is skipped unless
// S3_CONFORMANCE_ENDPOINT, S3_CONFORMANCE_BUCKET, and AWS_ACCESS_KEY_ID are set.
// Credentials come from the AWS SDK standard chain and are never printed.
func TestS3ConformanceConfiguredEndpoint(t *testing.T) {
	endpoint := os.Getenv("S3_CONFORMANCE_ENDPOINT")
	bucket := os.Getenv("S3_CONFORMANCE_BUCKET")
	if endpoint == "" || bucket == "" || os.Getenv("AWS_ACCESS_KEY_ID") == "" {
		t.Skip("S3_CONFORMANCE_ENDPOINT, S3_CONFORMANCE_BUCKET, and AWS_ACCESS_KEY_ID are required")
	}
	region := os.Getenv("S3_CONFORMANCE_REGION")
	if region == "" {
		region = "us-east-1"
	}
	store, err := newS3Store(context.Background(), config.Storage{
		Driver: config.StorageS3, S3Endpoint: endpoint, S3Region: region, S3Bucket: bucket,
		S3ForcePathStyle: os.Getenv("S3_CONFORMANCE_FORCE_PATH_STYLE") == "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	runS3Conformance(t, store, "conformance-"+randomS3Suffix(t))
}

// runS3Conformance asserts the object behavior the file service relies on:
// private objects, round trips, conditional create, not-found normalization,
// presigned PUT and GET with the requested expiry, and expired URL refusal.
// Every object it creates lives under prefix and is deleted at cleanup.
func runS3Conformance(t *testing.T, store *s3Store, prefix string) {
	t.Helper()
	ctx := context.Background()
	objectKey := prefix + "/object.txt"
	uploadKey := prefix + "/uploaded.txt"
	for _, key := range []string{objectKey, uploadKey} {
		t.Cleanup(func() {
			_, _ = store.client.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: &store.bucket, Key: &key})
		})
	}

	if err := store.Put(ctx, objectKey, strings.NewReader("hello"), 5, "text/plain"); err != nil {
		t.Fatalf("put: %v", err)
	}
	info, err := store.Head(ctx, objectKey)
	if err != nil || info.Size != 5 || info.ContentType != "text/plain" {
		t.Fatalf("head = %+v, error = %v", info, err)
	}
	reader, err := store.Get(ctx, objectKey)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	body, err := io.ReadAll(reader)
	reader.Close()
	if err != nil || string(body) != "hello" {
		t.Fatalf("get body = %q, error = %v", body, err)
	}

	if err := store.Put(ctx, objectKey, strings.NewReader("again"), 5, "text/plain"); err == nil {
		t.Fatal("overwrite succeeded; conditional create must reject an existing key")
	}

	missingKey := prefix + "/missing.txt"
	if _, err := store.Head(ctx, missingKey); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("head missing error = %v, want ErrObjectNotFound", err)
	}
	if _, err := store.Get(ctx, missingKey); !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("get missing error = %v, want ErrObjectNotFound", err)
	}

	getURL, err := store.PresignGet(ctx, objectKey, 5*time.Minute)
	if err != nil {
		t.Fatalf("presign get: %v", err)
	}
	parsedGet, err := url.Parse(getURL)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsedGet.Query().Get("X-Amz-Expires"); got != "300" {
		t.Fatalf("presigned GET X-Amz-Expires = %q, want 300", got)
	}
	if status, body := conformanceRequest(t, http.MethodGet, getURL, nil, nil); status != http.StatusOK || body != "hello" {
		t.Fatalf("presigned GET status = %d, body = %q", status, body)
	}

	unsigned := *parsedGet
	unsigned.RawQuery = ""
	if status, body := conformanceRequest(t, http.MethodGet, unsigned.String(), nil, nil); status < 400 || body == "hello" {
		t.Fatalf("unsigned GET status = %d; objects must stay private", status)
	}

	shortURL, err := store.PresignGet(ctx, objectKey, time.Second)
	if err != nil {
		t.Fatalf("presign short get: %v", err)
	}
	time.Sleep(2100 * time.Millisecond)
	if status, body := conformanceRequest(t, http.MethodGet, shortURL, nil, nil); status < 400 || body == "hello" {
		t.Fatalf("expired presigned GET status = %d; expiry must be enforced", status)
	}

	upload, err := store.PresignPut(ctx, uploadKey, 4, "text/plain", 5*time.Minute)
	if err != nil {
		t.Fatalf("presign put: %v", err)
	}
	headers := make(http.Header, len(upload.Headers))
	for name, value := range upload.Headers {
		headers.Set(name, value)
	}
	if status, _ := conformanceRequest(t, http.MethodPut, upload.URL, headers, strings.NewReader("data")); status/100 != 2 {
		t.Fatalf("presigned PUT status = %d, headers = %v", status, upload.Headers)
	}
	uploaded, err := store.Head(ctx, uploadKey)
	if err != nil || uploaded.Size != 4 {
		t.Fatalf("uploaded head = %+v, error = %v", uploaded, err)
	}
}

// conformanceRequest performs one plain HTTP request with the given headers and
// returns its status and body. It does not follow redirects.
func conformanceRequest(t *testing.T, method, rawURL string, headers http.Header, body io.Reader) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), method, rawURL, body)
	if err != nil {
		t.Fatal(err)
	}
	for name, values := range headers {
		req.Header[name] = values
	}
	if size, ok := body.(*strings.Reader); ok {
		req.ContentLength = int64(size.Len())
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		// url.Error embeds the full URL, which can carry a signature. Report only the cause.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		t.Fatalf("%s request failed: %v", method, err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(payload)
}

func randomS3Suffix(t *testing.T) string {
	t.Helper()
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(suffix[:])
}
