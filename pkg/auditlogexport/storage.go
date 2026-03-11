package auditlogexport

import (
	"context"
	"fmt"
	"io"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/storage/blob"
)

// StorageProvider defines the interface for all storage providers.
type StorageProvider interface {
	// Test tests if the storage provider is working.
	Test(ctx context.Context, config types.StorageConfig) error

	// Upload uploads the given data to the storage provider.
	Upload(ctx context.Context, config types.StorageConfig, bucket, key string, data io.Reader) error
}

// CredentialProvider defines the interface for credential management.
type CredentialProvider interface {
	GetStorageConfig(ctx context.Context) (*types.StorageConfig, error)
}

// blobStoreAdapter adapts blob.BlobStore to the audit log StorageProvider interface.
// It creates a new BlobStore per call since the audit log system passes config per-call.
type blobStoreAdapter struct {
	providerType types.StorageProviderType
}

func (a *blobStoreAdapter) Test(ctx context.Context, config types.StorageConfig) error {
	store, err := blob.New(a.providerType, config)
	if err != nil {
		return err
	}
	return store.Test(ctx)
}

func (a *blobStoreAdapter) Upload(ctx context.Context, config types.StorageConfig, bucket, key string, data io.Reader) error {
	store, err := blob.New(a.providerType, config)
	if err != nil {
		return err
	}
	return store.Upload(ctx, bucket, key, data)
}

// NewStorageProvider creates a storage provider instance based on the provider type.
func NewStorageProvider(providerType types.StorageProviderType, _ CredentialProvider) (StorageProvider, error) {
	switch providerType {
	case types.StorageProviderS3,
		types.StorageProviderGCS,
		types.StorageProviderAzureBlob,
		types.StorageProviderCustomS3:
		return &blobStoreAdapter{providerType: providerType}, nil
	default:
		return nil, fmt.Errorf("unsupported storage provider: %s", providerType)
	}
}
