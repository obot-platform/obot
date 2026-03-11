package blob

import (
	"context"
	"fmt"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/obot-platform/obot/apiclient/types"
)

// AzureStore implements BlobStore for Azure Blob Storage.
type AzureStore struct {
	config types.AzureConfig
}

// NewAzureStore creates a new Azure blob store.
func NewAzureStore(config types.AzureConfig) (*AzureStore, error) {
	if config.StorageAccount == "" {
		return nil, fmt.Errorf("storage account is required for Azure storage")
	}
	hasClientID := config.ClientID != ""
	hasTenantID := config.TenantID != ""
	hasClientSecret := config.ClientSecret != ""
	if hasClientID || hasTenantID || hasClientSecret {
		if !hasClientID || !hasTenantID || !hasClientSecret {
			return nil, fmt.Errorf("all of client ID, tenant ID, and client secret must be provided together for Azure storage")
		}
	}
	return &AzureStore{config: config}, nil
}

func (a *AzureStore) Upload(ctx context.Context, bucket, key string, data io.Reader) error {
	log.Debugf("Azure upload: bucket=%s key=%s", bucket, key)
	client, err := a.createClient()
	if err != nil {
		return err
	}

	_, err = client.UploadStream(ctx, bucket, key, data, &azblob.UploadStreamOptions{})
	if err != nil {
		return fmt.Errorf("failed to upload to Azure Blob Storage: %w", err)
	}
	return nil
}

func (a *AzureStore) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	log.Debugf("Azure download: bucket=%s key=%s", bucket, key)
	client, err := a.createClient()
	if err != nil {
		return nil, err
	}

	resp, err := client.DownloadStream(ctx, bucket, key, &azblob.DownloadStreamOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to download from Azure Blob Storage: %w", err)
	}
	return resp.Body, nil
}

func (a *AzureStore) Delete(ctx context.Context, bucket, key string) error {
	log.Debugf("Azure delete: bucket=%s key=%s", bucket, key)
	client, err := a.createClient()
	if err != nil {
		return err
	}

	_, err = client.DeleteBlob(ctx, bucket, key, &azblob.DeleteBlobOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete from Azure Blob Storage: %w", err)
	}
	return nil
}

func (a *AzureStore) Test(ctx context.Context) error {
	client, err := a.createClient()
	if err != nil {
		return fmt.Errorf("failed to create Azure Blob client: %w", err)
	}

	pager := client.NewListContainersPager(&azblob.ListContainersOptions{})
	if pager.More() {
		if _, err = pager.NextPage(ctx); err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}
	}
	return nil
}

func (a *AzureStore) createClient() (*azblob.Client, error) {
	var cred azcore.TokenCredential
	var err error
	if a.config.ClientID != "" && a.config.TenantID != "" && a.config.ClientSecret != "" {
		cred, err = azidentity.NewClientSecretCredential(
			a.config.TenantID,
			a.config.ClientID,
			a.config.ClientSecret,
			nil,
		)
	} else {
		cred, err = azidentity.NewDefaultAzureCredential(nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credentials: %w", err)
	}

	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net", a.config.StorageAccount)
	return azblob.NewClient(serviceURL, cred, nil)
}
