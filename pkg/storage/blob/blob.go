package blob

import (
	"context"
	"fmt"
	"io"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
)

var log = logger.Package()

// BlobStore provides a unified interface for cloud object storage operations.
type BlobStore interface {
	// Upload writes data to the given bucket and key.
	Upload(ctx context.Context, bucket, key string, data io.Reader) error

	// Download retrieves the object at the given bucket and key.
	// The caller is responsible for closing the returned ReadCloser.
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)

	// Delete removes the object at the given bucket and key.
	Delete(ctx context.Context, bucket, key string) error

	// Test verifies that the store is configured correctly and reachable.
	Test(ctx context.Context) error
}

// New creates a BlobStore for the given provider type and config.
func New(providerType types.StorageProviderType, config types.StorageConfig) (BlobStore, error) {
	log.Infof("Creating blob store: provider=%s", providerType)

	switch providerType {
	case types.StorageProviderS3:
		if config.S3Config == nil {
			return nil, fmt.Errorf("s3 configuration is required")
		}
		return NewS3Store(*config.S3Config)
	case types.StorageProviderGCS:
		if config.GCSConfig == nil {
			return nil, fmt.Errorf("GCS configuration is required")
		}
		return NewGCSStore(*config.GCSConfig)
	case types.StorageProviderAzureBlob:
		if config.AzureConfig == nil {
			return nil, fmt.Errorf("azure configuration is required")
		}
		return NewAzureStore(*config.AzureConfig)
	case types.StorageProviderCustomS3:
		if config.CustomS3Config == nil {
			return nil, fmt.Errorf("custom S3 configuration is required")
		}
		return NewCustomS3Store(*config.CustomS3Config)
	default:
		return nil, fmt.Errorf("unsupported storage provider: %s", providerType)
	}
}
