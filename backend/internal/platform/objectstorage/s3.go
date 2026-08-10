package objectstorage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Config struct {
	Region          string
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	ForcePathStyle  bool
	DefaultBucket   string
}

type S3Client struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
	now       func() time.Time
}

func NewS3Client(ctx context.Context, cfg S3Config, now func() time.Time) (*S3Client, error) {
	cfg.Region = strings.TrimSpace(cfg.Region)
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if strings.TrimSpace(cfg.AccessKeyID) == "" || strings.TrimSpace(cfg.SecretAccessKey) == "" {
		return nil, fmt.Errorf("object storage access key and secret key are required")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, err
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if endpoint := strings.TrimSpace(cfg.Endpoint); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
		options.UsePathStyle = cfg.ForcePathStyle
	})
	if now == nil {
		now = time.Now
	}
	return &S3Client{client: client, presigner: s3.NewPresignClient(client), bucket: strings.TrimSpace(cfg.DefaultBucket), now: now}, nil
}

func (c *S3Client) Check(ctx context.Context) error {
	bucket, err := c.bucketName("")
	if err != nil {
		return err
	}
	_, err = c.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	return err
}

func (c *S3Client) CreateMultipartUpload(ctx context.Context, input CreateMultipartUploadInput) (CreateMultipartUploadOutput, error) {
	bucket, err := c.bucketName(input.Bucket)
	if err != nil {
		return CreateMultipartUploadOutput{}, err
	}
	key, err := objectKey(input.Key)
	if err != nil {
		return CreateMultipartUploadOutput{}, err
	}
	output, err := c.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: optionalString(input.ContentType),
		Metadata:    input.Metadata,
	})
	if err != nil {
		return CreateMultipartUploadOutput{}, err
	}
	return CreateMultipartUploadOutput{UploadID: aws.ToString(output.UploadId)}, nil
}

func (c *S3Client) PresignUploadPart(ctx context.Context, input PresignUploadPartInput) (PresignedRequest, error) {
	bucket, err := c.bucketName(input.Bucket)
	if err != nil {
		return PresignedRequest{}, err
	}
	key, err := objectKey(input.Key)
	if err != nil {
		return PresignedRequest{}, err
	}
	if strings.TrimSpace(input.UploadID) == "" {
		return PresignedRequest{}, fmt.Errorf("upload id is required")
	}
	if input.PartNumber < 1 {
		return PresignedRequest{}, fmt.Errorf("part number must be positive")
	}
	expires := input.ExpiresIn
	if expires <= 0 {
		expires = 15 * time.Minute
	}
	request, err := c.presigner.PresignUploadPart(ctx, &s3.UploadPartInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		UploadId:      aws.String(input.UploadID),
		PartNumber:    aws.Int32(input.PartNumber),
		ContentLength: optionalContentLength(input.ContentBytes),
	}, func(options *s3.PresignOptions) {
		options.Expires = expires
	})
	if err != nil {
		return PresignedRequest{}, err
	}
	if err = ValidatePresignedURL(request.URL); err != nil {
		return PresignedRequest{}, err
	}
	return PresignedRequest{Method: request.Method, URL: request.URL, Headers: headerMap(request.SignedHeader), ExpiresAt: c.now().UTC().Add(expires)}, nil
}

func (c *S3Client) CompleteMultipartUpload(ctx context.Context, input CompleteMultipartUploadInput) error {
	bucket, err := c.bucketName(input.Bucket)
	if err != nil {
		return err
	}
	key, err := objectKey(input.Key)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.UploadID) == "" {
		return fmt.Errorf("upload id is required")
	}
	parts := make([]types.CompletedPart, 0, len(input.Parts))
	for _, part := range input.Parts {
		if part.PartNumber < 1 || strings.TrimSpace(part.ETag) == "" {
			return fmt.Errorf("completed part must include part number and etag")
		}
		parts = append(parts, types.CompletedPart{PartNumber: aws.Int32(part.PartNumber), ETag: aws.String(part.ETag)})
	}
	_, err = c.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(bucket),
		Key:             aws.String(key),
		UploadId:        aws.String(input.UploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	})
	return err
}

func (c *S3Client) AbortMultipartUpload(ctx context.Context, input AbortMultipartUploadInput) error {
	bucket, err := c.bucketName(input.Bucket)
	if err != nil {
		return err
	}
	key, err := objectKey(input.Key)
	if err != nil {
		return err
	}
	if strings.TrimSpace(input.UploadID) == "" {
		return fmt.Errorf("upload id is required")
	}
	_, err = c.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{Bucket: aws.String(bucket), Key: aws.String(key), UploadId: aws.String(input.UploadID)})
	return err
}

func (c *S3Client) PresignGetObject(ctx context.Context, input PresignGetObjectInput) (PresignedRequest, error) {
	bucket, err := c.bucketName(input.Bucket)
	if err != nil {
		return PresignedRequest{}, err
	}
	key, err := objectKey(input.Key)
	if err != nil {
		return PresignedRequest{}, err
	}
	expires := input.ExpiresIn
	if expires <= 0 {
		expires = 15 * time.Minute
	}
	request, err := c.presigner.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)}, func(options *s3.PresignOptions) {
		options.Expires = expires
	})
	if err != nil {
		return PresignedRequest{}, err
	}
	if err = ValidatePresignedURL(request.URL); err != nil {
		return PresignedRequest{}, err
	}
	return PresignedRequest{Method: request.Method, URL: request.URL, Headers: headerMap(request.SignedHeader), ExpiresAt: c.now().UTC().Add(expires)}, nil
}

func (c *S3Client) HeadObject(ctx context.Context, input HeadObjectInput) (HeadObjectOutput, error) {
	bucket, err := c.bucketName(input.Bucket)
	if err != nil {
		return HeadObjectOutput{}, err
	}
	key, err := objectKey(input.Key)
	if err != nil {
		return HeadObjectOutput{}, err
	}
	output, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return HeadObjectOutput{}, err
	}
	return HeadObjectOutput{SizeBytes: aws.ToInt64(output.ContentLength), ETag: strings.Trim(aws.ToString(output.ETag), `"`), ContentType: aws.ToString(output.ContentType), LastModified: aws.ToTime(output.LastModified)}, nil
}

func (c *S3Client) bucketName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = c.bucket
	}
	if value == "" {
		return "", fmt.Errorf("bucket is required")
	}
	return value, nil
}

func objectKey(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("object key is required")
	}
	return value, nil
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return aws.String(strings.TrimSpace(value))
}

func optionalContentLength(value int64) *int64 {
	if value <= 0 {
		return nil
	}
	return aws.Int64(value)
}

func headerMap(headers map[string][]string) map[string]string {
	result := make(map[string]string, len(headers))
	for key, values := range headers {
		if len(values) > 0 {
			result[key] = values[0]
		}
	}
	return result
}
