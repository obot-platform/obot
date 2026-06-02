package providers

import (
	"testing"

	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func TestConvertProviderToolRefMissingEntitlementIsConfigured(t *testing.T) {
	ref := providerToolReference(`{
		"requiredEntitlements": ["ENTITLEMENT"],
		"envVars": [{"name":"API_KEY"}]
	}`)

	status, err := ConvertProviderToolRef(ref, map[string]string{"API_KEY": "secret"}, testLicenseProvider(t))
	if err != nil {
		t.Fatalf("ConvertProviderToolRef() error = %v", err)
	}

	if !status.Configured {
		t.Fatal("Configured = false, want true when only required entitlement is missing")
	}
	if len(status.MissingConfigurationParameters) != 0 {
		t.Fatalf("MissingConfigurationParameters = %v, want empty", status.MissingConfigurationParameters)
	}
	if len(status.MissingEntitlements) != 1 || status.MissingEntitlements[0] != "ENTITLEMENT" {
		t.Fatalf("MissingEntitlements = %v, want [ENTITLEMENT]", status.MissingEntitlements)
	}
}

func TestConvertProviderToolRefConfiguredWhenRequirementsSatisfied(t *testing.T) {
	ref := providerToolReference(`{
		"envVars": [{"name":"API_KEY"}]
	}`)

	status, err := ConvertProviderToolRef(ref, map[string]string{"API_KEY": "secret"}, testLicenseProvider(t))
	if err != nil {
		t.Fatalf("ConvertProviderToolRef() error = %v", err)
	}

	if !status.Configured {
		t.Fatalf("Configured = false, want true; missing config=%v missing entitlements=%v", status.MissingConfigurationParameters, status.MissingEntitlements)
	}
}

func TestConvertProviderToolRefMissingConfigurationIsNotConfigured(t *testing.T) {
	ref := providerToolReference(`{
		"envVars": [{"name":"API_KEY"}]
	}`)

	status, err := ConvertProviderToolRef(ref, nil, testLicenseProvider(t))
	if err != nil {
		t.Fatalf("ConvertProviderToolRef() error = %v", err)
	}

	if status.Configured {
		t.Fatal("Configured = true, want false when required config is missing")
	}
	if len(status.MissingConfigurationParameters) != 1 || status.MissingConfigurationParameters[0] != "API_KEY" {
		t.Fatalf("MissingConfigurationParameters = %v, want [API_KEY]", status.MissingConfigurationParameters)
	}
}

func testLicenseProvider(t *testing.T) *license.KeygenProvider {
	t.Helper()

	provider, err := license.NewProvider(t.Context(), nil, license.Config{})
	if err != nil {
		t.Fatalf("NewProvider() error = %v", err)
	}
	return provider
}

func providerToolReference(providerMeta string) v1.ToolReference {
	return v1.ToolReference{
		Spec: v1.ToolReferenceSpec{
			Type: v1.ToolReferenceTypeModelProvider,
		},
		Status: v1.ToolReferenceStatus{
			Tool: &v1.ToolShortDescription{
				Metadata: map[string]string{
					"providerMeta": providerMeta,
				},
			},
		},
	}
}
