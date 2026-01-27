package auditlogexport

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3Provider_createClient_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    types.StorageConfig
		wantErr   bool
		errString string
	}{
		{
			name: "valid config with credentials",
			config: types.StorageConfig{
				S3Config: &types.S3Config{
					Region:          "us-west-2",
					AccessKeyID:     "test-key",
					SecretAccessKey: "test-secret",
				},
			},
			wantErr: false,
		},
		{
			name: "valid config without credentials (workload identity)",
			config: types.StorageConfig{
				S3Config: &types.S3Config{
					Region: "us-east-1",
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with only access key",
			config: types.StorageConfig{
				S3Config: &types.S3Config{
					Region:      "eu-west-1",
					AccessKeyID: "test-key",
				},
			},
			wantErr: false,
		},
		{
			name: "valid config with only secret key",
			config: types.StorageConfig{
				S3Config: &types.S3Config{
					Region:          "ap-south-1",
					SecretAccessKey: "test-secret",
				},
			},
			wantErr: false,
		},
		{
			name: "missing S3 config",
			config: types.StorageConfig{
				S3Config: nil,
			},
			wantErr:   true,
			errString: "s3 configuration is required",
		},
		{
			name: "valid config with empty region",
			config: types.StorageConfig{
				S3Config: &types.S3Config{
					Region:          "",
					AccessKeyID:     "test-key",
					SecretAccessKey: "test-secret",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewS3Provider(nil)
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

func TestS3Provider_Test_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    types.StorageConfig
		wantErr   bool
		errString string
	}{
		{
			name: "missing S3 config",
			config: types.StorageConfig{
				S3Config: nil,
			},
			wantErr:   true,
			errString: "s3 configuration is required",
		},
		{
			name: "missing region",
			config: types.StorageConfig{
				S3Config: &types.S3Config{
					Region:          "",
					AccessKeyID:     "test-key",
					SecretAccessKey: "test-secret",
				},
			},
			wantErr:   true,
			errString: "region is required",
		},
		{
			name: "valid config with credentials",
			config: types.StorageConfig{
				S3Config: &types.S3Config{
					Region:          "us-west-2",
					AccessKeyID:     "test-key",
					SecretAccessKey: "test-secret",
				},
			},
			wantErr: true, // Will fail due to invalid credentials, but validates structure
		},
		{
			name: "valid config without credentials",
			config: types.StorageConfig{
				S3Config: &types.S3Config{
					Region: "us-east-1",
				},
			},
			wantErr: true, // Will fail due to no auth, but validates structure
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewS3Provider(nil)
			err := provider.Test(context.Background(), tt.config)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errString != "" {
					assert.Contains(t, err.Error(), tt.errString)
				}
			}
		})
	}
}

func TestNewS3Provider(t *testing.T) {
	provider := NewS3Provider(nil)
	require.NotNil(t, provider)
	assert.Nil(t, provider.credProvider)

	mockProvider := &mockCredentialProvider{}
	provider = NewS3Provider(mockProvider)
	require.NotNil(t, provider)
	assert.Equal(t, mockProvider, provider.credProvider)
}

// mockCredentialProvider is a test helper
type mockCredentialProvider struct{}

func (m *mockCredentialProvider) GetStorageConfig(ctx context.Context) (*types.StorageConfig, error) {
	return nil, nil
}
