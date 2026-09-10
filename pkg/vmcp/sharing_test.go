package vmcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestIsMultiUser(t *testing.T) {
	component := types.VMCPComponent{
		Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed}},
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
			Config: []types.MCPConfig{{Key: "TOKEN", Usage: types.Header}},
		}},
	}
	if !IsMultiUser(component) {
		t.Fatal("fixed configuration should share")
	}
	component.Configuration[0].Policy = types.VMCPConfigurationPolicyUserAllowed
	if !IsMultiUser(component) {
		t.Fatal("user headers should share")
	}
	component.ForceSingleUser = true
	if IsMultiUser(component) {
		t.Fatal("forceSingleUser ignored")
	}
	component.ForceSingleUser = false
	component.CatalogEntry.Manifest.Config = nil
	if IsMultiUser(component) {
		t.Fatal("unknown user input must not share")
	}
	component.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "TOKEN", Usage: types.Env}}
	if IsMultiUser(component) {
		t.Fatal("user environment input must not share")
	}
	component.Configuration[0].Policy = types.VMCPConfigurationPolicyProhibited
	if !IsMultiUser(component) {
		t.Fatal("prohibited configuration should share")
	}
}
