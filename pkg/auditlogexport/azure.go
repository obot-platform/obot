package auditlogexport

import (
	"context"
	"fmt"
	"io"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/obot-platform/obot/apiclient/types"
)

// AzureProvider implements StorageProvider for Azure Blob Storage
type AzureProvider struct {
	credProvider CredentialProvider
}

// NewAzureProvider creates a new Azure Blob Storage provider
func NewAzureProvider(credProvider CredentialProvider) *AzureProvider {
	return &AzureProvider{
		credProvider: credProvider,
	}
}

func (a *AzureProvider) Upload(ctx context.Context, config types.StorageConfig, bucket, key string, data io.Reader) error {
	client, err := a.createClient(config)
	if err != nil {
		return err
	}

	options := &azblob.UploadStreamOptions{}

	_, err = client.UploadStream(ctx, bucket, key, data, options)
	if err != nil {
		return fmt.Errorf("failed to upload to Azure Blob Storage: %w", err)
	}

	return nil
}

func (a *AzureProvider) Test(ctx context.Context, config types.StorageConfig) error {
	client, err := a.createClient(config)
	if err != nil {
		return fmt.Errorf("failed to create Azure Blob client: %w", err)
	}

	pager := client.NewListContainersPager(&azblob.ListContainersOptions{})
	if pager.More() {
		_, err = pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list containers: %w", err)
		}
	}

	return nil
}

func (a *AzureProvider) createClient(config types.StorageConfig) (*azblob.Client, error) {
	azureConfig := config.AzureConfig
	if azureConfig == nil {
		return nil, fmt.Errorf("azure configuration is required")
	}

	var cred azcore.TokenCredential
	var err error
	if azureConfig.ClientID != "" || azureConfig.TenantID != "" || azureConfig.ClientSecret != "" {
		cred, err = azidentity.NewClientSecretCredential(
			azureConfig.TenantID,
			azureConfig.ClientID,
			azureConfig.ClientSecret,
			nil,
		)
	} else {
		cred, err = azidentity.NewDefaultAzureCredential(nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Azure credentials: %w", err)
	}

	// Construct the service URL
	serviceURL := fmt.Sprintf("https://%s.blob.core.windows.net", azureConfig.StorageAccount)

	return azblob.NewClient(serviceURL, cred, nil)
}
