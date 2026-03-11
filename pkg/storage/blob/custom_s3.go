package blob

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/obot-platform/obot/apiclient/types"
)

// CustomS3Store implements BlobStore for S3-compatible storage (MinIO, R2, etc.).
type CustomS3Store struct {
	config types.CustomS3Config
}

// NewCustomS3Store creates a new custom S3 blob store.
func NewCustomS3Store(config types.CustomS3Config) (*CustomS3Store, error) {
	if config.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required for custom S3 storage")
	}
	if config.Region == "" {
		return nil, fmt.Errorf("region is required for custom S3 storage")
	}
	if config.AccessKeyID == "" || config.SecretAccessKey == "" {
		return nil, fmt.Errorf("access key ID and secret access key are required for custom S3 storage")
	}
	return &CustomS3Store{config: config}, nil
}

func (c *CustomS3Store) Upload(ctx context.Context, bucket, key string, data io.Reader) error {
	log.Debugf("CustomS3 upload: endpoint=%s bucket=%s key=%s", c.config.Endpoint, bucket, key)
	client, err := c.createClient(ctx)
	if err != nil {
		return err
	}

	uploader := manager.NewUploader(client)
	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   data,
	})
	if err != nil {
		return fmt.Errorf("failed to upload to custom S3 storage: %w", err)
	}
	return nil
}

func (c *CustomS3Store) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	log.Debugf("CustomS3 download: endpoint=%s bucket=%s key=%s", c.config.Endpoint, bucket, key)
	client, err := c.createClient(ctx)
	if err != nil {
		return nil, err
	}

	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download from custom S3 storage: %w", err)
	}
	return output.Body, nil
}

func (c *CustomS3Store) Delete(ctx context.Context, bucket, key string) error {
	log.Debugf("CustomS3 delete: endpoint=%s bucket=%s key=%s", c.config.Endpoint, bucket, key)
	client, err := c.createClient(ctx)
	if err != nil {
		return err
	}

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete from custom S3 storage: %w", err)
	}
	return nil
}

// Test is a no-op for custom S3 storage as there is no standard way to test it.
func (c *CustomS3Store) Test(context.Context) error {
	return nil
}

func (c *CustomS3Store) createClient(ctx context.Context) (*s3.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	cfg.Credentials = credentials.NewStaticCredentialsProvider(
		c.config.AccessKeyID,
		c.config.SecretAccessKey,
		"",
	)
	cfg.Region = c.config.Region

	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(c.config.Endpoint)
		o.UsePathStyle = true
	}), nil
}
