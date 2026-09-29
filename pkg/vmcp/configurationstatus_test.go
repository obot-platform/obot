package vmcp

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
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

func TestMissingRequiredConfigurationFixedSecretBinding(t *testing.T) {
	component := types.VMCPComponent{
		ID: "gitlab",
		Configuration: []types.VMCPConfigurationPolicy{{
			Key:           "GITLAB_PERSONAL_ACCESS_TOKEN",
			Policy:        types.VMCPConfigurationPolicyFixed,
			SecretBinding: &types.MCPSecretBinding{Name: "gitlab-secret", Key: "gitlab_key"},
		}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Config: []types.MCPConfig{{Key: "GITLAB_PERSONAL_ACCESS_TOKEN", Required: true, Sensitive: true, Usage: types.Env}},
		}},
	}

	if got := MissingRequiredConfiguration(component, nil, false); len(got) != 0 {
		t.Fatalf("secret-bound configuration reported missing: %v", got)
	}

	config := ComponentConfig(component)
	if len(config) != 1 || config[0].SecretBinding == nil || *config[0].SecretBinding != *component.Configuration[0].SecretBinding {
		t.Fatalf("ComponentConfig() did not apply secret binding: %+v", config)
	}
	if component.CatalogEntry.Manifest.Config[0].SecretBinding != nil {
		t.Fatal("ComponentConfig() mutated the catalog entry snapshot")
	}
}

// A connection must still read as synced after storage drops its empty Config.
func TestConnectionConfigurationSyncedSurvivesStorageRoundTrip(t *testing.T) {
	instance := v1.VMCPInstance{Status: v1.VMCPInstanceStatus{UserConfigurationHash: "saved"}}
	for _, tc := range []struct {
		name      string
		component types.VMCPComponent
	}{
		{
			name:      "no configuration",
			component: types.VMCPComponent{ID: "one"},
		},
		{
			name: "only fixed configuration",
			component: types.VMCPComponent{
				ID:            "one",
				Configuration: []types.VMCPConfigurationPolicy{{Key: "FIXED", Policy: types.VMCPConfigurationPolicyFixed}},
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Config: []types.MCPConfig{{Key: "FIXED", Required: true, Usage: types.Header}},
				}},
			},
		},
		{
			name: "user configuration with empty options",
			component: types.VMCPComponent{
				ID:            "one",
				Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyUserAllowed}},
				CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
					Config: []types.MCPConfig{{Key: "TOKEN", Required: true, Usage: types.Header, Options: []types.MCPConfigurationOption{}}},
				}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Record the sync the way the MCPServerInstance controller does.
			config := ConnectionConfiguration(tc.component)
			connection := v1.MCPServerInstance{
				Spec:   v1.MCPServerInstanceSpec{Config: config},
				Status: v1.MCPServerInstanceStatus{VMCPConfigurationHash: ConnectionConfigurationHash(config, instance)},
			}

			stored, err := json.Marshal(connection)
			if err != nil {
				t.Fatal(err)
			}
			var loaded v1.MCPServerInstance
			if err := json.Unmarshal(stored, &loaded); err != nil {
				t.Fatal(err)
			}

			if !ConnectionConfigurationSynced(loaded, tc.component, instance) {
				t.Fatalf("stored connection %s is not synced", stored)
			}
		})
	}
}
