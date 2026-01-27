package auditlogexport

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGCSProvider_createClient_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    types.StorageConfig
		wantErr   bool
		errString string
	}{
		{
			name: "missing GCS config",
			config: types.StorageConfig{
				GCSConfig: nil,
			},
			wantErr:   true,
			errString: "GCS configuration is required",
		},
		{
			name: "valid config with service account JSON",
			config: types.StorageConfig{
				GCSConfig: &types.GCSConfig{
					ServiceAccountJSON: `{"type":"service_account"}`,
				},
			},
			wantErr: false, // Client creation succeeds, validation happens on actual use
		},
		{
			name: "valid config without service account (workload identity)",
			config: types.StorageConfig{
				GCSConfig: &types.GCSConfig{
					ServiceAccountJSON: "",
				},
			},
			wantErr: false,
		},
		{
			name: "empty GCS config",
			config: types.StorageConfig{
				GCSConfig: &types.GCSConfig{},
			},
			wantErr: false, // Empty config is valid for workload identity
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewGCSProvider(nil)
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
			client.Close()
		})
	}
}

func TestGCSProvider_Test_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    types.StorageConfig
		wantErr   bool
		errString string
	}{
		{
			name: "missing GCS config",
			config: types.StorageConfig{
				GCSConfig: nil,
			},
			wantErr:   true,
			errString: "GCS configuration is required",
		},
		{
			name: "empty GCS config",
			config: types.StorageConfig{
				GCSConfig: &types.GCSConfig{},
			},
			wantErr: false,
		},
		{
			name: "GCS config with service account JSON",
			config: types.StorageConfig{
				GCSConfig: &types.GCSConfig{
					ServiceAccountJSON: `{"type":"service_account"}`,
				},
			},
			wantErr: false, // Client creation succeeds, Test() just creates and closes client
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewGCSProvider(nil)
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

func TestNewGCSProvider(t *testing.T) {
	provider := NewGCSProvider(nil)
	require.NotNil(t, provider)
	assert.Nil(t, provider.credProvider)

	mockProvider := &mockCredentialProvider{}
	provider = NewGCSProvider(mockProvider)
	require.NotNil(t, provider)
	assert.Equal(t, mockProvider, provider.credProvider)
}
