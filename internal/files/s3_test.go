package files

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

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
