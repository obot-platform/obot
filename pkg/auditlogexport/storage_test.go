package auditlogexport

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStorageProvider(t *testing.T) {
	tests := []struct {
		name         string
		providerType types.StorageProviderType
		wantErr      bool
		checkType    func(StorageProvider) bool
	}{
		{
			name:         "S3 provider",
			providerType: types.StorageProviderS3,
			wantErr:      false,
			checkType: func(p StorageProvider) bool {
				_, ok := p.(*S3Provider)
				return ok
			},
		},
		{
			name:         "GCS provider",
			providerType: types.StorageProviderGCS,
			wantErr:      false,
			checkType: func(p StorageProvider) bool {
				_, ok := p.(*GCSProvider)
				return ok
			},
		},
		{
			name:         "Azure provider",
			providerType: types.StorageProviderAzureBlob,
			wantErr:      false,
			checkType: func(p StorageProvider) bool {
				_, ok := p.(*AzureProvider)
				return ok
			},
		},
		{
			name:         "Custom S3 provider",
			providerType: types.StorageProviderCustomS3,
			wantErr:      false,
			checkType: func(p StorageProvider) bool {
				_, ok := p.(*CustomS3Provider)
				return ok
			},
		},
		{
			name:         "unsupported provider",
			providerType: types.StorageProviderType("invalid"),
			wantErr:      true,
		},
		{
			name:         "empty provider type",
			providerType: types.StorageProviderType(""),
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewStorageProvider(tt.providerType, nil)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, provider)
				assert.Contains(t, err.Error(), "unsupported storage provider")
				return
			}

			require.NoError(t, err)
			require.NotNil(t, provider)
			assert.True(t, tt.checkType(provider), "Expected type check to pass, got: %s", reflect.TypeOf(provider))
		})
	}
}

func TestNewStorageProvider_TypeNames(t *testing.T) {
	// Additional test to verify exact type names
	tests := []struct {
		providerType types.StorageProviderType
		wantTypeName string
	}{
		{types.StorageProviderS3, "*auditlogexport.S3Provider"},
		{types.StorageProviderGCS, "*auditlogexport.GCSProvider"},
		{types.StorageProviderAzureBlob, "*auditlogexport.AzureProvider"},
		{types.StorageProviderCustomS3, "*auditlogexport.CustomS3Provider"},
	}

	for _, tt := range tests {
		t.Run(string(tt.providerType), func(t *testing.T) {
			provider, err := NewStorageProvider(tt.providerType, nil)
			require.NoError(t, err)
			require.NotNil(t, provider)

			actualType := fmt.Sprintf("%T", provider)
			assert.Equal(t, tt.wantTypeName, actualType)
		})
	}
}
