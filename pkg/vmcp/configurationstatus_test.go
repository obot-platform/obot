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
			Config: []types.MCPConfig{
				{Key: "FIXED", Required: true, Usage: types.Env},
				{Key: "USER", Required: true, Usage: types.File},
				{Key: "PROHIBITED", Required: true, Usage: types.Env},
				{Key: "LITERAL", Required: true, Value: "static", Usage: types.Env},
				{Key: "OPTIONAL", Usage: types.Env},
				{Key: "HEADER", Required: true, Usage: types.Header},
			},
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
