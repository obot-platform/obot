package auditlogexport

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomS3Provider_createClient_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    types.StorageConfig
		wantErr   bool
		errString string
	}{
		{
			name: "valid config",
			config: types.StorageConfig{
				CustomS3Config: &types.CustomS3Config{
					Endpoint:        "http://localhost:9000",
					Region:          "us-east-1",
					AccessKeyID:     "test-key",
					SecretAccessKey: "test-secret",
				},
			},
			wantErr: false,
		},
		{
			name: "missing CustomS3 config",
			config: types.StorageConfig{
				CustomS3Config: nil,
			},
			wantErr:   true,
			errString: "custom S3 configuration is required",
		},
		{
			name: "missing endpoint",
			config: types.StorageConfig{
				CustomS3Config: &types.CustomS3Config{
					Endpoint:        "",
					Region:          "us-east-1",
					AccessKeyID:     "test-key",
					SecretAccessKey: "test-secret",
				},
			},
			wantErr:   true,
			errString: "endpoint is required",
		},
		{
			name: "missing region",
			config: types.StorageConfig{
				CustomS3Config: &types.CustomS3Config{
					Endpoint:        "http://localhost:9000",
					Region:          "",
					AccessKeyID:     "test-key",
					SecretAccessKey: "test-secret",
				},
			},
			wantErr:   true,
			errString: "region is required",
		},
		{
			name: "missing access key ID",
			config: types.StorageConfig{
				CustomS3Config: &types.CustomS3Config{
					Endpoint:        "http://localhost:9000",
					Region:          "us-east-1",
					AccessKeyID:     "",
					SecretAccessKey: "test-secret",
				},
			},
			wantErr:   true,
			errString: "access key ID and secret access key are required",
		},
		{
			name: "missing secret access key",
			config: types.StorageConfig{
				CustomS3Config: &types.CustomS3Config{
					Endpoint:        "http://localhost:9000",
					Region:          "us-east-1",
					AccessKeyID:     "test-key",
					SecretAccessKey: "",
				},
			},
			wantErr:   true,
			errString: "access key ID and secret access key are required",
		},
		{
			name: "missing both access keys",
			config: types.StorageConfig{
				CustomS3Config: &types.CustomS3Config{
					Endpoint:        "http://localhost:9000",
					Region:          "us-east-1",
					AccessKeyID:     "",
					SecretAccessKey: "",
				},
			},
			wantErr:   true,
			errString: "access key ID and secret access key are required",
		},
		{
			name: "all fields missing",
			config: types.StorageConfig{
				CustomS3Config: &types.CustomS3Config{},
			},
			wantErr:   true,
			errString: "endpoint is required",
		},
		{
			name: "valid config with HTTPS endpoint",
			config: types.StorageConfig{
				CustomS3Config: &types.CustomS3Config{
					Endpoint:        "https://minio.example.com",
					Region:          "eu-west-1",
					AccessKeyID:     "test-key",
					SecretAccessKey: "test-secret",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewCustomS3Provider(nil)
			client, err := provider.createClient(context.Background(), tt.config)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, client)
				if tt.errString != "" {
					assert.Contains(t, err.Error(), tt.errString)
				}
				return
			}

			require.NoError(t, err)
			require.NotNil(t, client)
		})
	}
}

func TestCustomS3Provider_Test(t *testing.T) {
	// Test is a no-op, so it should always succeed
	provider := NewCustomS3Provider(nil)

	tests := []struct {
		name   string
		config types.StorageConfig
	}{
		{
			name: "nil config",
			config: types.StorageConfig{
				CustomS3Config: nil,
			},
		},
		{
			name: "valid config",
			config: types.StorageConfig{
				CustomS3Config: &types.CustomS3Config{
					Endpoint:        "http://localhost:9000",
					Region:          "us-east-1",
					AccessKeyID:     "test-key",
					SecretAccessKey: "test-secret",
				},
			},
		},
		{
			name: "empty config",
			config: types.StorageConfig{
				CustomS3Config: &types.CustomS3Config{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.Test(context.Background(), tt.config)
			assert.NoError(t, err, "Test should always succeed as it's a no-op")
		})
	}
}

func TestNewCustomS3Provider(t *testing.T) {
	provider := NewCustomS3Provider(nil)
	require.NotNil(t, provider)
	assert.Nil(t, provider.credProvider)

	mockProvider := &mockCredentialProvider{}
	provider = NewCustomS3Provider(mockProvider)
	require.NotNil(t, provider)
	assert.Equal(t, mockProvider, provider.credProvider)
}
