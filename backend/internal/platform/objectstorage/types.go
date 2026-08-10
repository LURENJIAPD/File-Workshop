package objectstorage

import (
	"context"
	"errors"
	"net/url"
	"time"
)

var ErrDisabled = errors.New("object storage is disabled")

type Part struct {
	PartNumber int32
	ETag       string
}

type CreateMultipartUploadInput struct {
	Bucket      string
	Key         string
	ContentType string
	Metadata    map[string]string
}

type CreateMultipartUploadOutput struct {
	UploadID string
}

type PresignUploadPartInput struct {
	Bucket       string
	Key          string
	UploadID     string
	PartNumber   int32
	ExpiresIn    time.Duration
	ContentBytes int64
}

type PresignedRequest struct {
	Method    string
	URL       string
	Headers   map[string]string
	ExpiresAt time.Time
}

type CompleteMultipartUploadInput struct {
	Bucket   string
	Key      string
	UploadID string
	Parts    []Part
}

type AbortMultipartUploadInput struct {
	Bucket   string
	Key      string
	UploadID string
}

type PresignGetObjectInput struct {
	Bucket    string
	Key       string
	ExpiresIn time.Duration
}

type HeadObjectInput struct {
	Bucket string
	Key    string
}

type HeadObjectOutput struct {
	SizeBytes    int64
	ETag         string
	ContentType  string
	LastModified time.Time
}

type Client interface {
	Check(context.Context) error
	CreateMultipartUpload(context.Context, CreateMultipartUploadInput) (CreateMultipartUploadOutput, error)
	PresignUploadPart(context.Context, PresignUploadPartInput) (PresignedRequest, error)
	CompleteMultipartUpload(context.Context, CompleteMultipartUploadInput) error
	AbortMultipartUpload(context.Context, AbortMultipartUploadInput) error
	PresignGetObject(context.Context, PresignGetObjectInput) (PresignedRequest, error)
	HeadObject(context.Context, HeadObjectInput) (HeadObjectOutput, error)
}

func ValidatePresignedURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("presigned URL must use http or https")
	}
	if parsed.Host == "" {
		return errors.New("presigned URL host is required")
	}
	return nil
}
