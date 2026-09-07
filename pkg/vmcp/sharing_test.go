package vmcp

import (
	"github.com/obot-platform/obot/apiclient/types"
	"testing"
)

func TestIsMultiUser(t *testing.T) {
	manifest := types.VMCPManifest{Components: []types.VMCPComponent{{
		Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			RemoteConfig: &types.RemoteCatalogConfig{Headers: []types.MCPHeader{{Key: "TOKEN"}}},
		}},
	}}}
	if !IsMultiUser(manifest) {
		t.Fatal("fixed configuration should share")
	}
	manifest.Components[0].Configuration[0].Policy = types.VMCPConfigurationPolicyUserAllowed
	if !IsMultiUser(manifest) {
		t.Fatal("user headers should share")
	}
	manifest.ForceSingleUser = true
	if IsMultiUser(manifest) {
		t.Fatal("forceSingleUser ignored")
	}
	manifest.ForceSingleUser = false
	manifest.Components[0].CatalogEntry.Manifest.RemoteConfig = nil
	if IsMultiUser(manifest) {
		t.Fatal("unknown user input must not share")
	}
	manifest.Components[0].CatalogEntry.Manifest.Env = []types.MCPEnv{{Key: "TOKEN"}}
	if IsMultiUser(manifest) {
		t.Fatal("user environment input must not share")
	}
	manifest.Components[0].Configuration[0].Policy = types.VMCPConfigurationPolicyProhibited
	if !IsMultiUser(manifest) {
		t.Fatal("prohibited configuration should share")
	}
}
