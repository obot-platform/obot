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
	apitypes "github.com/obot-platform/obot/apiclient/types"
)

type CustomS3Provider struct {
	credProvider CredentialProvider
}

func NewCustomS3Provider(credProvider CredentialProvider) *CustomS3Provider {
	return &CustomS3Provider{
		credProvider: credProvider,
	}
}

func (c *CustomS3Provider) Upload(ctx context.Context, config apitypes.StorageConfig, bucket, key string, data io.Reader) error {
	client, err := c.createClient(ctx, config)
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
	if err != nil {
		return fmt.Errorf("failed to upload to custom S3 storage: %w", err)
	}

	return nil
}

// Test is a no-op for custom S3 storage as there is no way to test it without uploading a files
func (c *CustomS3Provider) Test(context.Context, apitypes.StorageConfig) error {
	return nil
}

func (c *CustomS3Provider) createClient(ctx context.Context, storageConfig apitypes.StorageConfig) (*s3.Client, error) {
	customS3Config := storageConfig.CustomS3Config
	if customS3Config == nil {
		return nil, fmt.Errorf("custom S3 configuration is required")
	}

	// Validate required fields
	if customS3Config.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required for custom S3 storage")
	}
	if customS3Config.Region == "" {
		return nil, fmt.Errorf("region is required for custom S3 storage")
	}
	if customS3Config.AccessKeyID == "" || customS3Config.SecretAccessKey == "" {
		return nil, fmt.Errorf("access key ID and secret access key are required for custom S3 storage")
	}

	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	cfg.Credentials = credentials.NewStaticCredentialsProvider(
		customS3Config.AccessKeyID,
		customS3Config.SecretAccessKey,
		"",
	)

	cfg.Region = customS3Config.Region

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(customS3Config.Endpoint)
		o.UsePathStyle = true
	})

	return client, nil
}
