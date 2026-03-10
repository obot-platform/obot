package blob

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

// Compile-time interface compliance checks.
var (
	_ BlobStore = (*S3Store)(nil)
	_ BlobStore = (*GCSStore)(nil)
	_ BlobStore = (*AzureStore)(nil)
	_ BlobStore = (*CustomS3Store)(nil)
)

func TestNew_ValidProviders(t *testing.T) {
	tests := []struct {
		name         string
		providerType types.StorageProviderType
		config       types.StorageConfig
	}{
		{
			name:         "s3",
			providerType: types.StorageProviderS3,
			config:       types.StorageConfig{S3Config: &types.S3Config{Region: "us-east-1"}},
		},
		{
			name:         "gcs",
			providerType: types.StorageProviderGCS,
			config:       types.StorageConfig{GCSConfig: &types.GCSConfig{}},
		},
		{
			name:         "azure",
			providerType: types.StorageProviderAzureBlob,
			config:       types.StorageConfig{AzureConfig: &types.AzureConfig{StorageAccount: "test"}},
		},
		{
			name:         "custom_s3",
			providerType: types.StorageProviderCustomS3,
			config: types.StorageConfig{CustomS3Config: &types.CustomS3Config{
				Endpoint:        "http://localhost:9000",
				Region:          "us-east-1",
				AccessKeyID:     "key",
				SecretAccessKey: "secret",
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := New(tt.providerType, tt.config)
			if err != nil {
				t.Fatalf("New() returned error: %v", err)
			}
			if store == nil {
				t.Fatal("New() returned nil store")
			}
		})
	}
}

func TestNew_UnsupportedProvider(t *testing.T) {
	_, err := New("unsupported", types.StorageConfig{})
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}

func TestNew_MissingConfig(t *testing.T) {
	tests := []struct {
		name         string
		providerType types.StorageProviderType
		config       types.StorageConfig
	}{
		{
			name:         "s3 nil config",
			providerType: types.StorageProviderS3,
			config:       types.StorageConfig{},
		},
		{
			name:         "gcs nil config",
			providerType: types.StorageProviderGCS,
			config:       types.StorageConfig{},
		},
		{
			name:         "azure nil config",
			providerType: types.StorageProviderAzureBlob,
			config:       types.StorageConfig{},
		},
		{
			name:         "custom_s3 nil config",
			providerType: types.StorageProviderCustomS3,
			config:       types.StorageConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.providerType, tt.config)
			if err == nil {
				t.Fatal("expected error for missing config")
			}
		})
	}
}

func TestNew_ValidationErrors(t *testing.T) {
	tests := []struct {
		name         string
		providerType types.StorageProviderType
		config       types.StorageConfig
	}{
		{
			name:         "s3 missing region",
			providerType: types.StorageProviderS3,
			config:       types.StorageConfig{S3Config: &types.S3Config{}},
		},
		{
			name:         "azure missing storage account",
			providerType: types.StorageProviderAzureBlob,
			config:       types.StorageConfig{AzureConfig: &types.AzureConfig{}},
		},
		{
			name:         "custom_s3 missing endpoint",
			providerType: types.StorageProviderCustomS3,
			config:       types.StorageConfig{CustomS3Config: &types.CustomS3Config{Region: "us-east-1", AccessKeyID: "k", SecretAccessKey: "s"}},
		},
		{
			name:         "custom_s3 missing region",
			providerType: types.StorageProviderCustomS3,
			config:       types.StorageConfig{CustomS3Config: &types.CustomS3Config{Endpoint: "http://localhost", AccessKeyID: "k", SecretAccessKey: "s"}},
		},
		{
			name:         "custom_s3 missing credentials",
			providerType: types.StorageProviderCustomS3,
			config:       types.StorageConfig{CustomS3Config: &types.CustomS3Config{Endpoint: "http://localhost", Region: "us-east-1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.providerType, tt.config)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

