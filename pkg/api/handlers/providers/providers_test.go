package providers

import (
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAuthProviderStatus(t *testing.T) {
	tests := []struct {
		name         string
		authProvider v1.AuthProvider
		cred         map[string]string
		want         types.CommonProviderStatus
	}{
		{
			name: "configured when requirements are satisfied",
			authProvider: authProvider(
				[]types.ProviderConfigurationParameter{{Name: "API_KEY"}},
				nil,
				nil,
			),
			cred: map[string]string{"API_KEY": "secret"},
			want: types.CommonProviderStatus{
				Configured: true,
			},
		},
		{
			name: "missing entitlement does not make provider unconfigured",
			authProvider: authProvider(
				[]types.ProviderConfigurationParameter{{Name: "API_KEY"}},
				nil,
				[]string{"ENTITLEMENT"},
			),
			cred: map[string]string{"API_KEY": "secret"},
			want: types.CommonProviderStatus{
				Configured:          true,
				MissingEntitlements: []string{"ENTITLEMENT"},
			},
		},
		{
			name: "missing credential value is not configured",
			authProvider: authProvider(
				[]types.ProviderConfigurationParameter{{Name: "API_KEY"}},
				nil,
				nil,
			),
			cred: map[string]string{},
			want: types.CommonProviderStatus{
				Configured:                     false,
				MissingConfigurationParameters: []string{"API_KEY"},
			},
		},
		{
			name: "nil credential uses status missing configuration",
			authProvider: authProvider(
				[]types.ProviderConfigurationParameter{{Name: "API_KEY"}},
				[]string{"API_KEY"},
				nil,
			),
			cred: nil,
			want: types.CommonProviderStatus{
				Configured:                     false,
				MissingConfigurationParameters: []string{"API_KEY"},
			},
		},
		{
			name: "nil credential falls back to required configuration when status is empty",
			authProvider: authProvider(
				[]types.ProviderConfigurationParameter{{Name: "API_KEY"}},
				nil,
				nil,
			),
			cred: nil,
			want: types.CommonProviderStatus{
				Configured:                     false,
				MissingConfigurationParameters: []string{"API_KEY"},
			},
		},
	}

	licenseProvider := testLicenseProvider(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AuthProviderStatus(t.Context(), tt.authProvider, tt.cred, licenseProvider)
			if err != nil {
				t.Fatalf("AuthProviderStatus() error = %v", err)
			}

			assertCommonProviderStatus(t, got.CommonProviderStatus, tt.want)
		})
	}
}

func TestModelProviderStatus(t *testing.T) {
	tests := []struct {
		name          string
		modelProvider v1.ModelProvider
		cred          map[string]string
		want          types.ModelProviderStatus
	}{
		{
			name: "configured when requirements are satisfied and models are current",
			modelProvider: modelProvider(
				[]types.ProviderConfigurationParameter{{Name: "API_KEY"}},
				nil,
				nil,
				2,
				2,
			),
			cred: map[string]string{"API_KEY": "secret"},
			want: types.ModelProviderStatus{
				CommonProviderStatus: types.CommonProviderStatus{Configured: true},
				ModelsBackPopulated:  new(true),
			},
		},
		{
			name: "configured when requirements are satisfied and models are stale",
			modelProvider: modelProvider(
				[]types.ProviderConfigurationParameter{{Name: "API_KEY"}},
				nil,
				nil,
				2,
				1,
			),
			cred: map[string]string{"API_KEY": "secret"},
			want: types.ModelProviderStatus{
				CommonProviderStatus: types.CommonProviderStatus{Configured: true},
				ModelsBackPopulated:  new(false),
			},
		},
		{
			name: "missing entitlement does not make provider unconfigured",
			modelProvider: modelProvider(
				[]types.ProviderConfigurationParameter{{Name: "API_KEY"}},
				nil,
				[]string{"ENTITLEMENT"},
				2,
				2,
			),
			cred: map[string]string{"API_KEY": "secret"},
			want: types.ModelProviderStatus{
				CommonProviderStatus: types.CommonProviderStatus{
					Configured:          true,
					MissingEntitlements: []string{"ENTITLEMENT"},
				},
				ModelsBackPopulated: new(true),
			},
		},
		{
			name: "missing credential value is not configured and skips model population status",
			modelProvider: modelProvider(
				[]types.ProviderConfigurationParameter{{Name: "API_KEY"}},
				nil,
				nil,
				2,
				2,
			),
			cred: map[string]string{},
			want: types.ModelProviderStatus{
				CommonProviderStatus: types.CommonProviderStatus{
					Configured:                     false,
					MissingConfigurationParameters: []string{"API_KEY"},
				},
			},
		},
		{
			name: "nil credential uses status missing configuration",
			modelProvider: modelProvider(
				[]types.ProviderConfigurationParameter{{Name: "API_KEY"}},
				[]string{"API_KEY"},
				nil,
				2,
				2,
			),
			cred: nil,
			want: types.ModelProviderStatus{
				CommonProviderStatus: types.CommonProviderStatus{
					Configured:                     false,
					MissingConfigurationParameters: []string{"API_KEY"},
				},
			},
		},
		{
			name: "nil credential falls back to required configuration when status is empty",
			modelProvider: modelProvider(
				[]types.ProviderConfigurationParameter{{Name: "API_KEY"}},
				nil,
				nil,
				2,
				2,
			),
			cred: nil,
			want: types.ModelProviderStatus{
				CommonProviderStatus: types.CommonProviderStatus{
					Configured:                     false,
					MissingConfigurationParameters: []string{"API_KEY"},
				},
			},
		},
	}

	licenseProvider := testLicenseProvider(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ModelProviderStatus(t.Context(), tt.modelProvider, tt.cred, licenseProvider)
			if err != nil {
				t.Fatalf("ModelProviderStatus() error = %v", err)
			}

			assertModelProviderStatus(t, got, tt.want)
		})
	}
}

func assertModelProviderStatus(t *testing.T, got *types.ModelProviderStatus, want types.ModelProviderStatus) {
	t.Helper()

	if got == nil || !reflect.DeepEqual(*got, want) {
		t.Fatalf("ModelProviderStatus = %#v, want %#v", got, want)
	}
}

func assertCommonProviderStatus(t *testing.T, got, want types.CommonProviderStatus) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CommonProviderStatus = %#v, want %#v", got, want)
	}
}

func testLicenseProvider(t *testing.T) *license.Provider {
	t.Helper()

	provider, err := license.NewProvider(t.Context(), nil, license.Config{})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	return provider
}

func authProvider(requiredConfig []types.ProviderConfigurationParameter, missingConfig []string, requiredEntitlements []string) v1.AuthProvider {
	statusPopulated := missingConfig != nil

	return v1.AuthProvider{
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: types.AuthProviderManifest{
				CommonProviderMetadata: types.CommonProviderMetadata{
					RequiredConfigurationParameters: requiredConfig,
					RequiredEntitlements:            requiredEntitlements,
				},
			},
		},
		Status: v1.AuthProviderStatus{
			Configured:                     statusPopulated,
			MissingConfigurationParameters: missingConfig,
		},
	}
}

func modelProvider(requiredConfig []types.ProviderConfigurationParameter, missingConfig []string, requiredEntitlements []string, generation int64, observedGeneration int64) v1.ModelProvider {
	statusPopulated := missingConfig != nil

	return v1.ModelProvider{
		ObjectMeta: metav1.ObjectMeta{
			Generation: generation,
		},
		Spec: v1.ModelProviderSpec{
			ModelProviderManifest: types.ModelProviderManifest{
				CommonProviderMetadata: types.CommonProviderMetadata{
					RequiredConfigurationParameters: requiredConfig,
					RequiredEntitlements:            requiredEntitlements,
				},
			},
		},
		Status: v1.ModelProviderStatus{
			Configured:                     statusPopulated,
			MissingConfigurationParameters: missingConfig,
			ObservedGeneration:             observedGeneration,
		},
	}
}
