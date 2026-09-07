package vmcp

import (
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestMissingRequiredConfiguration(t *testing.T) {
	component := types.VMCPComponent{
		ID: "one",
		Configuration: []types.VMCPConfigurationPolicy{
			{Key: "FIXED", Policy: types.VMCPConfigurationPolicyFixed},
			{Key: "USER", Policy: types.VMCPConfigurationPolicyUserAllowed},
			{Key: "HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed},
		},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Env: []types.MCPEnv{
				{MCPHeader: types.MCPHeader{Key: "FIXED", Required: true}},
				{MCPHeader: types.MCPHeader{Key: "USER", Required: true}, File: true},
				{MCPHeader: types.MCPHeader{Key: "PROHIBITED", Required: true}},
				{MCPHeader: types.MCPHeader{Key: "LITERAL", Required: true, Value: "static"}},
				{MCPHeader: types.MCPHeader{Key: "OPTIONAL"}},
			},
			RemoteConfig: &types.RemoteCatalogConfig{Headers: []types.MCPHeader{{Key: "HEADER", Required: true}}},
		}},
	}
	values := map[string]string{ConfigurationKey(component.ID, "FIXED"): "secret", ConfigurationKey(component.ID, "PROHIBITED"): "stale"}
	if got := MissingRequiredConfiguration(component, values, false); !slices.Equal(got, []string{ConfigurationKey("one", "PROHIBITED")}) {
		t.Fatalf("administrator missing = %v", got)
	}
	if got := MissingRequiredConfiguration(component, values, true); !slices.Equal(got, []string{ConfigurationKey("one", "HEADER"), ConfigurationKey("one", "USER")}) {
		t.Fatalf("user missing = %v", got)
	}
	values[ConfigurationKey("one", "USER")] = "file contents"
	values[ConfigurationKey("one", "HEADER")] = "header secret"
	if got := MissingRequiredConfiguration(component, values, true); len(got) != 0 {
		t.Fatalf("configured inputs reported missing: %v", got)
	}
}
