package vmcp

import (
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestExtractStaticConfigurationPreservesExistingValues(t *testing.T) {
	manifest := types.VMCPManifest{Components: []types.VMCPComponent{{
		ID: "component-id",
		Configuration: []types.VMCPConfigurationPolicy{
			{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed, Value: "new-token"},
			{Key: "REGION", Policy: types.VMCPConfigurationPolicyFixed},
			{Key: "USER", Policy: types.VMCPConfigurationPolicyUserAllowed, Value: "ignored"},
		},
	}}}

	got := ExtractStaticConfiguration(&manifest, map[string]string{
		ConfigurationKey("component-id", "TOKEN"):   "old-token",
		ConfigurationKey("component-id", "REGION"):  "old-region",
		ConfigurationKey("component-id", "REMOVED"): "old-removed",
	})
	want := map[string]string{
		ConfigurationKey("component-id", "TOKEN"):  "new-token",
		ConfigurationKey("component-id", "REGION"): "old-region",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractStaticConfiguration() = %v, want %v", got, want)
	}
	for _, policy := range manifest.Components[0].Configuration {
		if policy.Value != "" {
			t.Fatalf("configuration %q value was not cleared", policy.Key)
		}
	}
}

func TestConfigurationKeyRoundTrip(t *testing.T) {
	key := ConfigurationKey("component-id", "HEADER.with.periods")
	componentID, configurationKey, ok := ParseConfigurationKey(key)
	if !ok || componentID != "component-id" || configurationKey != "HEADER.with.periods" {
		t.Fatalf("ParseConfigurationKey(%q) = %q, %q, %v", key, componentID, configurationKey, ok)
	}
}

func TestValidateAndEncodeUserConfiguration(t *testing.T) {
	manifest := types.VMCPManifest{Components: []types.VMCPComponent{{
		ID:   "component-id",
		Name: "component",
		Configuration: []types.VMCPConfigurationPolicy{
			{Key: "USER", Policy: types.VMCPConfigurationPolicyUserAllowed},
			{Key: "FIXED", Policy: types.VMCPConfigurationPolicyFixed},
		},
	}}}
	got, err := ValidateAndEncodeUserConfiguration(manifest, types.VMCPConfiguration{
		Components: map[string]map[string]string{"component-id": {"USER": " value "}},
	})
	if err != nil {
		t.Fatalf("ValidateAndEncodeUserConfiguration() error = %v", err)
	}
	if got[ConfigurationKey("component-id", "USER")] != "value" {
		t.Fatalf("encoded configuration = %v", got)
	}

	_, err = ValidateAndEncodeUserConfiguration(manifest, types.VMCPConfiguration{
		Components: map[string]map[string]string{"component-id": {"FIXED": "override"}},
	})
	if err == nil {
		t.Fatal("ValidateAndEncodeUserConfiguration() accepted fixed configuration")
	}
}

// TestValidateAndEncodeUserConfigurationRefusesServerOwnedValues checks that a user can't set a catalog-static or
// Secret-bound value, even when a policy (for example from catalog YAML) marks its key userAllowed.
func TestValidateAndEncodeUserConfigurationRefusesServerOwnedValues(t *testing.T) {
	manifest := types.VMCPManifest{Components: []types.VMCPComponent{{
		ID:   "component-id",
		Name: "component",
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Config: []types.MCPConfig{
			{Key: "USER"},
			{Key: "STATIC", Static: true},
			{Key: "BOUND", SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "token"}},
		}}},
		Configuration: []types.VMCPConfigurationPolicy{
			{Key: "USER", Policy: types.VMCPConfigurationPolicyUserAllowed},
			{Key: "STATIC", Policy: types.VMCPConfigurationPolicyUserAllowed},
			{Key: "BOUND", Policy: types.VMCPConfigurationPolicyUserAllowed},
		},
	}}}
	for _, test := range []struct {
		name    string
		key     string
		wantErr bool
	}{
		{
			name: "user-allowed value",
			key:  "USER",
		},
		{
			name:    "catalog-static value",
			key:     "STATIC",
			wantErr: true,
		},
		{
			name:    "Secret-bound value",
			key:     "BOUND",
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := ValidateAndEncodeUserConfiguration(manifest, types.VMCPConfiguration{
				Components: map[string]map[string]string{"component-id": {test.key: "value"}},
			})
			if test.wantErr {
				if err == nil {
					t.Fatalf("ValidateAndEncodeUserConfiguration() accepted %s: %v", test.key, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateAndEncodeUserConfiguration() error = %v", err)
			}
			if got[ConfigurationKey("component-id", test.key)] != "value" {
				t.Fatalf("encoded configuration = %v", got)
			}
		})
	}
}

func TestExtractStaticConfigurationOmitsSecretBoundValues(t *testing.T) {
	manifest := types.VMCPManifest{Components: []types.VMCPComponent{{
		ID: "component-id",
		Configuration: []types.VMCPConfigurationPolicy{{
			Key:           "TOKEN",
			Policy:        types.VMCPConfigurationPolicyFixed,
			SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "token"},
		}},
	}}}

	got := ExtractStaticConfiguration(&manifest, map[string]string{
		ConfigurationKey("component-id", "TOKEN"): "old-clear-text",
	})
	if len(got) != 0 {
		t.Fatalf("ExtractStaticConfiguration() = %v, want no stored values", got)
	}
	if manifest.Components[0].Configuration[0].SecretBinding == nil {
		t.Fatal("ExtractStaticConfiguration() removed the secret binding")
	}
}
