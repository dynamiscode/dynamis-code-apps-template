package files

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

// The presigned URL host must match BrowserOrigin, or the CSP blocks the upload.
func TestS3PresignedURLOriginMatchesBrowserOrigin(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "access")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")
	for _, storage := range []config.Storage{
		{Driver: config.StorageS3, S3Region: "us-east-1", S3Bucket: "structured-closet-bnlu44g", S3Endpoint: "https://t3.storageapi.dev"},
		{Driver: config.StorageS3, S3Region: "us-east-1", S3Bucket: "structured-closet-bnlu44g", S3Endpoint: "https://t3.storageapi.dev", S3ForcePathStyle: true},
		{Driver: config.StorageS3, S3Region: "us-east-1", S3Bucket: "files"},
		{Driver: config.StorageS3, S3Region: "cn-north-1", S3Bucket: "files"},
	} {
		store, err := newS3Store(context.Background(), storage)
		if err != nil {
			t.Fatal(err)
		}
		upload, err := store.PresignPut(context.Background(), "workspace/file", 5, "text/plain", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := url.Parse(upload.URL)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := parsed.Scheme+"://"+parsed.Host, storage.BrowserOrigin(); got != want {
			t.Fatalf("presigned origin = %q, BrowserOrigin() = %q", got, want)
		}
	}
}

func TestS3PresignPutReturnsRequiredHeaders(t *testing.T) {
	awsConfig := aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("access", "secret", ""),
	}
	client := s3.NewFromConfig(awsConfig)
	store := &s3Store{client: client, presign: s3.NewPresignClient(client), bucket: "bucket"}

	upload, err := store.PresignPut(context.Background(), "workspace/file", 5, "text/plain", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if upload.URL == "" || upload.Headers["Content-Length"] != "5" || upload.Headers["Content-Type"] != "text/plain" || upload.Headers["If-None-Match"] != "*" {
		t.Fatalf("presigned upload = %+v", upload)
	}
	if _, ok := upload.Headers["Host"]; ok {
		t.Fatal("presigned upload must not expose Host header")
	}
}

func TestS3NotFoundRecognizesGenericProviderErrors(t *testing.T) {
	if !isS3NotFound(&smithy.GenericAPIError{Code: "NotFound"}) {
		t.Fatal("generic NotFound error was not normalized")
	}
}
