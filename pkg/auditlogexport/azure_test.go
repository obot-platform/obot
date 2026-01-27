package auditlogexport

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureProvider_createClient_Validation(t *testing.T) {
	tests := []struct {
		name      string
		config    types.StorageConfig
		wantErr   bool
		errString string
	}{
		{
			name: "missing Azure config",
			config: types.StorageConfig{
				AzureConfig: nil,
			},
			wantErr:   true,
			errString: "azure configuration is required",
		},
		{
			name: "valid config with client credentials",
			config: types.StorageConfig{
				AzureConfig: &types.AzureConfig{
					StorageAccount: "teststorage",
					ClientID:       "test-client-id",
					TenantID:       "test-tenant-id",
					ClientSecret:   "test-secret",
				},
			},
			wantErr: false, // Client creation succeeds, actual auth happens later
		},
		{
			name: "invalid config with partial credentials",
			config: types.StorageConfig{
				AzureConfig: &types.AzureConfig{
					StorageAccount: "teststorage",
					ClientID:       "test-client-id",
					TenantID:       "",
					ClientSecret:   "",
				},
			},
			wantErr:   true,
			errString: "secret can't be empty string", // Azure SDK validates this
		},
		{
			name: "valid config without credentials (workload identity)",
			config: types.StorageConfig{
				AzureConfig: &types.AzureConfig{
					StorageAccount: "teststorage",
				},
			},
			wantErr: false, // Client creation succeeds with workload identity
		},
		{
			name: "empty storage account",
			config: types.StorageConfig{
				AzureConfig: &types.AzureConfig{
					StorageAccount: "",
					ClientID:       "test-client-id",
					TenantID:       "test-tenant-id",
					ClientSecret:   "test-secret",
				},
			},
			wantErr: false, // Client creation succeeds, empty account will fail on actual operations
		},
		{
			name: "empty Azure config",
			config: types.StorageConfig{
				AzureConfig: &types.AzureConfig{},
			},
			wantErr: false, // Client creation succeeds
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewAzureProvider(nil)
			client, err := provider.createClient(tt.config)

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

func TestNewAzureProvider(t *testing.T) {
	provider := NewAzureProvider(nil)
	require.NotNil(t, provider)
	assert.Nil(t, provider.credProvider)

	mockProvider := &mockCredentialProvider{}
	provider = NewAzureProvider(mockProvider)
	require.NotNil(t, provider)
	assert.Equal(t, mockProvider, provider.credProvider)
}
