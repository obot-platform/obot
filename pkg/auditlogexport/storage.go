package auditlogexport

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

// StorageProvider defines the interface for all storage providers
type StorageProvider interface {
	// Test tests if the storage provider is working
	Test(ctx context.Context, config types.StorageConfig) error

	// Upload uploads the given data to the storage provider
	Upload(ctx context.Context, config types.StorageConfig, bucket, key string, data io.Reader) error
}

// FileMetadata contains information about a stored file
type FileMetadata struct {
	Size         int64
	LastModified *time.Time
	ContentType  string
	ETag         string
}

// CredentialProvider defines the interface for credential management
type CredentialProvider interface {
	GetStorageConfig(ctx context.Context) (*types.StorageConfig, error)
}

type StorageConfig struct {
	S3Config  *S3Config
	GCSConfig *GCSConfig
	R2Config  *R2Config
}

type S3Config struct {
	Region string

	AccessKeyID     string
	SecretAccessKey string
}

type GCSConfig struct {
	Bucket    string
	KeyPrefix string

	ServiceAccountJSON string
}

type R2Config struct {
	Bucket    string
	KeyPrefix string

	AccessKeyID     string
	SecretAccessKey string
}

// NewStorageProvider creates a storage provider instance based on the provider type
func NewStorageProvider(providerType types.StorageProviderType, credProvider CredentialProvider) (StorageProvider, error) {
	switch providerType {
	case types.StorageProviderS3:
		return NewS3Provider(credProvider), nil
	case types.StorageProviderGCS:
		return NewGCSProvider(credProvider), nil
	case types.StorageProviderAzureBlob:
		return NewAzureProvider(credProvider), nil
	case types.StorageProviderCustomS3:
		return NewCustomS3Provider(credProvider), nil
	default:
		return nil, fmt.Errorf("unsupported storage provider: %s", providerType)
	}
}
