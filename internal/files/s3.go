package files

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"example.com/dynamis-code/apps-template/internal/platform/config"
)

type s3Store struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func newS3Store(ctx context.Context, cfg config.Storage) (*s3Store, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.S3Region))
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = cfg.S3ForcePathStyle
		if cfg.S3Endpoint != "" {
			options.BaseEndpoint = aws.String(cfg.S3Endpoint)
		}
	})
	return &s3Store{
		client: client, presign: s3.NewPresignClient(client), bucket: cfg.S3Bucket,
	}, nil
}

func (s *s3Store) Put(ctx context.Context, key string, source io.Reader, size int64, contentType string) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: source,
		ContentLength: aws.Int64(size), ContentType: aws.String(contentType), IfNoneMatch: aws.String("*"),
	})
	return err
}

func (s *s3Store) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	return result.Body, nil
}

func (s *s3Store) Head(ctx context.Context, key string) (ObjectInfo, error) {
	result, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	})
	if err != nil {
		if isS3NotFound(err) {
			return ObjectInfo{}, ErrObjectNotFound
		}
		return ObjectInfo{}, err
	}
	if result.ContentLength == nil {
		return ObjectInfo{}, ErrSizeMismatch
	}
	contentType := ""
	if result.ContentType != nil {
		contentType = *result.ContentType
	}
	return ObjectInfo{Size: *result.ContentLength, ContentType: contentType}, nil
}

func isS3NotFound(err error) bool {
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == http.StatusNotFound {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NotFound"
	}
	return false
}

func (s *s3Store) PresignPut(ctx context.Context, key string, size int64, contentType string, ttl time.Duration) (PresignedUpload, error) {
	result, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
		ContentLength: aws.Int64(size), ContentType: aws.String(contentType), IfNoneMatch: aws.String("*"),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return PresignedUpload{}, err
	}
	headers := make(map[string]string, len(result.SignedHeader))
	for name, values := range result.SignedHeader {
		if strings.EqualFold(name, "Host") || len(values) == 0 {
			continue
		}
		headers[name] = values[0]
	}
	return PresignedUpload{URL: result.URL, Headers: headers}, nil
}

func (*s3Store) SupportsPresignedPut() bool { return true }

func (s *s3Store) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	result, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", err
	}
	return result.URL, nil
}
