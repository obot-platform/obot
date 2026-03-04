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
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/obot-platform/obot/apiclient/types"
)

// S3Store implements BlobStore for Amazon S3.
type S3Store struct {
	config types.S3Config
}

// NewS3Store creates a new S3 blob store.
func NewS3Store(config types.S3Config) (*S3Store, error) {
	if config.Region == "" {
		return nil, fmt.Errorf("region is required for S3 storage")
	}
	return &S3Store{config: config}, nil
}

func (s *S3Store) Upload(ctx context.Context, bucket, key string, data io.Reader) error {
	client, err := s.createClient(ctx)
	if err != nil {
		return err
	}

	uploader := manager.NewUploader(client)
	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   data,
	})
	return err
}

func (s *S3Store) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	client, err := s.createClient(ctx)
	if err != nil {
		return nil, err
	}

	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download from S3: %w", err)
	}
	return output.Body, nil
}

func (s *S3Store) Delete(ctx context.Context, bucket, key string) error {
	client, err := s.createClient(ctx)
	if err != nil {
		return err
	}

	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete from S3: %w", err)
	}
	return nil
}

func (s *S3Store) Test(ctx context.Context) error {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}

	if s.config.AccessKeyID != "" || s.config.SecretAccessKey != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider(
			s.config.AccessKeyID,
			s.config.SecretAccessKey,
			"",
		)
	}

	if s.config.Region != "" {
		cfg.Region = s.config.Region
	}

	client := sts.NewFromConfig(cfg)
	_, err = client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to test S3 credentials: %w", err)
	}
	return nil
}

func (s *S3Store) createClient(ctx context.Context) (*s3.Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	if s.config.AccessKeyID != "" || s.config.SecretAccessKey != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider(
			s.config.AccessKeyID,
			s.config.SecretAccessKey,
			"",
		)
	}

	if s.config.Region != "" {
		cfg.Region = s.config.Region
	}

	return s3.NewFromConfig(cfg), nil
}
