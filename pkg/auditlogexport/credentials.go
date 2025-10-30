package auditlogexport

import (
	"context"
	"errors"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
)

var (
	storeageCredentialsContext = "audit-log-export-storage-global"
	storageCredentialsName     = "audit-log-export-storage-credentials"
)

type GPTScriptCredentialProvider struct {
	gptClient *gptscript.GPTScript
}

func NewGPTScriptCredentialProvider(gptClient *gptscript.GPTScript) *GPTScriptCredentialProvider {
	return &GPTScriptCredentialProvider{
		gptClient: gptClient,
	}
}

func (g *GPTScriptCredentialProvider) GetStorageConfig(ctx context.Context) (*types.StorageConfig, error) {
	credential, err := g.gptClient.RevealCredential(ctx, []string{storeageCredentialsContext}, storageCredentialsName)
	if err != nil {
		return nil, err
	}

	provider := credential.Env["provider"]

	storageConfig := &types.StorageConfig{}
	switch provider {
	case string(types.StorageProviderS3):
		storageConfig.S3Config = &types.S3Config{
			Region:          credential.Env["region"],
			AccessKeyID:     credential.Env["access_key_id"],
			SecretAccessKey: credential.Env["secret_access_key"],
		}
	case string(types.StorageProviderGCS):
		storageConfig.GCSConfig = &types.GCSConfig{
			ServiceAccountJSON: credential.Env["service_account_json"],
		}
	case string(types.StorageProviderAzureBlob):
		storageConfig.AzureConfig = &types.AzureConfig{
			StorageAccount: credential.Env["storage_account"],
			ClientID:       credential.Env["client_id"],
			TenantID:       credential.Env["tenant_id"],
			ClientSecret:   credential.Env["client_secret"],
		}

	case string(types.StorageProviderCustomS3):
		storageConfig.CustomS3Config = &types.CustomS3Config{
			Endpoint:        credential.Env["endpoint"],
			Region:          credential.Env["region"],
			AccessKeyID:     credential.Env["access_key_id"],
			SecretAccessKey: credential.Env["secret_access_key"],
		}
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", provider)
	}

	return storageConfig, nil
}

func (g *GPTScriptCredentialProvider) StoreCredentials(ctx context.Context, config types.StorageProviderConfigInput) error {
	credentialData := make(map[string]string)
	provider := config.Provider

	existingCredentialData := make(map[string]string)
	credential, err := g.gptClient.RevealCredential(ctx, []string{storeageCredentialsContext}, storageCredentialsName)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return err
	} else if err == nil {
		existingCredentialData = credential.Env
	}

	switch provider {
	case types.StorageProviderS3:
		if config.S3Config.Region != "" {
			credentialData["region"] = config.S3Config.Region
		} else {
			credentialData["region"] = existingCredentialData["region"]
		}
		if !config.UseWorkloadIdentity {
			if config.S3Config.AccessKeyID != "" {
				credentialData["access_key_id"] = config.S3Config.AccessKeyID
			} else {
				credentialData["access_key_id"] = existingCredentialData["access_key_id"]
			}
			if config.S3Config.SecretAccessKey != "" {
				credentialData["secret_access_key"] = config.S3Config.SecretAccessKey
			} else {
				credentialData["secret_access_key"] = existingCredentialData["secret_access_key"]
			}
		} else {
			credentialData["access_key_id"] = ""
			credentialData["secret_access_key"] = ""
		}
		credentialData["provider"] = string(types.StorageProviderS3)
	case types.StorageProviderGCS:
		if !config.UseWorkloadIdentity {
			if config.GCSConfig.ServiceAccountJSON != "" {
				credentialData["service_account_json"] = config.GCSConfig.ServiceAccountJSON
			} else {
				credentialData["service_account_json"] = existingCredentialData["service_account_json"]
			}
		} else {
			credentialData["service_account_json"] = ""
		}
		credentialData["provider"] = string(types.StorageProviderGCS)
	case types.StorageProviderAzureBlob:
		if config.AzureConfig.StorageAccount != "" {
			credentialData["storage_account"] = config.AzureConfig.StorageAccount
		} else {
			credentialData["storage_account"] = existingCredentialData["storage_account"]
		}
		if !config.UseWorkloadIdentity {
			if config.AzureConfig.ClientID != "" {
				credentialData["client_id"] = config.AzureConfig.ClientID
			} else {
				credentialData["client_id"] = existingCredentialData["client_id"]
			}
			if config.AzureConfig.TenantID != "" {
				credentialData["tenant_id"] = config.AzureConfig.TenantID
			} else {
				credentialData["tenant_id"] = existingCredentialData["tenant_id"]
			}
			if config.AzureConfig.ClientSecret != "" {
				credentialData["client_secret"] = config.AzureConfig.ClientSecret
			} else {
				credentialData["client_secret"] = existingCredentialData["client_secret"]
			}
		} else {
			credentialData["client_id"] = ""
			credentialData["tenant_id"] = ""
			credentialData["client_secret"] = ""
		}
		credentialData["provider"] = string(types.StorageProviderAzureBlob)
	case types.StorageProviderCustomS3:
		if config.CustomS3Config.Endpoint != "" {
			credentialData["endpoint"] = config.CustomS3Config.Endpoint
		} else {
			credentialData["endpoint"] = existingCredentialData["endpoint"]
		}
		if config.CustomS3Config.Region != "" {
			credentialData["region"] = config.CustomS3Config.Region
		} else {
			credentialData["region"] = existingCredentialData["region"]
		}
		if config.CustomS3Config.AccessKeyID != "" {
			credentialData["access_key_id"] = config.CustomS3Config.AccessKeyID
		} else {
			credentialData["access_key_id"] = existingCredentialData["access_key_id"]
		}
		if config.CustomS3Config.SecretAccessKey != "" {
			credentialData["secret_access_key"] = config.CustomS3Config.SecretAccessKey
		} else {
			credentialData["secret_access_key"] = existingCredentialData["secret_access_key"]
		}
		credentialData["provider"] = string(types.StorageProviderCustomS3)
	}

	return g.gptClient.CreateCredential(ctx, gptscript.Credential{
		Type:     gptscript.CredentialTypeTool,
		Context:  storeageCredentialsContext,
		ToolName: storageCredentialsName,
		Env:      credentialData,
	})
}

func (g *GPTScriptCredentialProvider) DeleteCredentials(ctx context.Context) error {
	return g.gptClient.DeleteCredential(ctx, storeageCredentialsContext, storageCredentialsName)
}

func (g *GPTScriptCredentialProvider) TestCredentials(ctx context.Context, config types.StorageConfig) error {
	var provider types.StorageProviderType
	if config.S3Config != nil {
		provider = types.StorageProviderS3
	} else if config.GCSConfig != nil {
		provider = types.StorageProviderGCS
	}

	p, err := NewStorageProvider(provider, g)
	if err != nil {
		return err
	}

	err = p.Test(ctx, config)
	if err != nil {
		return err
	}

	return err
}
