package vmcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestIsMultiUser(t *testing.T) {
	manifest := types.VMCPManifest{Components: []types.VMCPComponent{{
		Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Config: []types.MCPConfig{{Key: "TOKEN", Usage: types.Header}},
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
	manifest.Components[0].CatalogEntry.Manifest.Config = nil
	if IsMultiUser(manifest) {
		t.Fatal("unknown user input must not share")
	}
	manifest.Components[0].CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "TOKEN", Usage: types.Env}}
	if IsMultiUser(manifest) {
		t.Fatal("user environment input must not share")
	}
	manifest.Components[0].Configuration[0].Policy = types.VMCPConfigurationPolicyProhibited
	if !IsMultiUser(manifest) {
		t.Fatal("prohibited configuration should share")
	}
}
