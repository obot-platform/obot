package auditlogexport

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	apitypes "github.com/obot-platform/obot/apiclient/types"
)

// S3Provider implements StorageProvider for Amazon S3
type S3Provider struct {
	credProvider CredentialProvider
}

// NewS3Provider creates a new S3 storage provider
func NewS3Provider(credProvider CredentialProvider) *S3Provider {
	return &S3Provider{
		credProvider: credProvider,
	}
}

func (s *S3Provider) Upload(ctx context.Context, config apitypes.StorageConfig, bucket, key string, data io.Reader) error {
	client, err := s.createClient(ctx, config)
	if err != nil {
		return err
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   data,
	}

	uploader := manager.NewUploader(client)
	_, err = uploader.Upload(ctx, input)
	return err
}

func (s *S3Provider) Test(ctx context.Context, storageConfig apitypes.StorageConfig) error {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}

	s3Config := storageConfig.S3Config
	if s3Config == nil {
		return fmt.Errorf("s3 configuration is required")
	}

	if s3Config.Region == "" {
		return fmt.Errorf("region is required")
	}

	if s3Config.AccessKeyID != "" || s3Config.SecretAccessKey != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider(
			s3Config.AccessKeyID,
			s3Config.SecretAccessKey,
			"",
		)
	}

	if s3Config.Region != "" {
		cfg.Region = s3Config.Region
	}

	client := sts.NewFromConfig(cfg)
	_, err = client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to test S3 credentials: %w", err)
	}

	return nil
}

func (s *S3Provider) createClient(ctx context.Context, storageConfig apitypes.StorageConfig) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}

	s3Config := storageConfig.S3Config
	if s3Config == nil {
		return nil, fmt.Errorf("s3 configuration is required")
	}

	// Configure credentials if provided
	if s3Config.AccessKeyID != "" || s3Config.SecretAccessKey != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider(
			s3Config.AccessKeyID,
			s3Config.SecretAccessKey,
			"",
		)
	}

	// Configure region if provided
	if s3Config.Region != "" {
		cfg.Region = s3Config.Region
	}

	// Create S3 client with custom options
	client := s3.NewFromConfig(cfg)

	return client, nil
}
