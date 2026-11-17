// Package objectstore stores private object bytes outside application processes.
package objectstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Store is the object lifecycle needed by upload and account-deletion services.
type Store interface {
	Put(context.Context, string, io.Reader, int64, string) error
	Get(context.Context, string) (io.ReadCloser, string, error)
	Delete(context.Context, string) error
}

type S3 struct {
	client *s3.Client
	bucket string
}

var _ Store = (*S3)(nil)

// FromEnv returns nil when object storage is entirely unconfigured. Partial
// configuration fails startup instead of silently writing to an ephemeral disk.
func FromEnv(lookup func(string) string) (Store, error) {
	endpoint := lookup("S3_ENDPOINT")
	bucket := lookup("S3_BUCKET")
	region := lookup("S3_REGION")
	accessKey := lookup("S3_ACCESS_KEY_ID")
	secretKey := lookup("S3_SECRET_ACCESS_KEY")
	if endpoint == "" && bucket == "" && accessKey == "" && secretKey == "" {
		return nil, nil
	}
	if bucket == "" || region == "" || accessKey == "" || secretKey == "" {
		return nil, fmt.Errorf("S3_BUCKET, S3_REGION and S3 credentials must be configured together")
	}
	if strings.ContainsAny(bucket, "/\\ \t\n") {
		return nil, fmt.Errorf("invalid S3_BUCKET")
	}
	if endpoint != "" {
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return nil, fmt.Errorf("S3_ENDPOINT must be an HTTP(S) origin")
		}
		if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, fmt.Errorf("S3_ENDPOINT must not contain credentials, a path or a query")
		}
	}
	client := s3.New(s3.Options{
		Region:                     region,
		Credentials:                aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		HTTPClient:                 &http.Client{Timeout: 45 * time.Second},
		UsePathStyle:               endpoint != "",
		BaseEndpoint:               optionalString(endpoint),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &S3{client: client, bucket: bucket}, nil
}

func (s *S3) Put(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	if body == nil || size <= 0 || contentType == "" {
		return fmt.Errorf("object requires a body, positive length and content type")
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), Body: body,
		ContentLength: aws.Int64(size), ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("store object: %w", err)
	}
	return nil
}

func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, string, error) {
	if err := validateKey(key); err != nil {
		return nil, "", err
	}
	result, err := s.client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return nil, "", fmt.Errorf("read object: %w", err)
	}
	return result.Body, aws.ToString(result.ContentType), nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	if err := validateKey(key); err != nil {
		return err
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key)})
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return aws.String(strings.TrimRight(value, "/"))
}

func validateKey(key string) error {
	if key == "" || len(key) > 1024 || strings.HasPrefix(key, "/") || strings.ContainsAny(key, "\\\x00\r\n") {
		return fmt.Errorf("invalid object key")
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("invalid object key segment")
		}
	}
	return nil
}
