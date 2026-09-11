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
