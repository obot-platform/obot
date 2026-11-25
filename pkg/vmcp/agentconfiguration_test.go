package vmcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
)

// fixedAndCatalogComponent returns a component with every kind of value: a required and an optional value a user
// would enter, a required value the vMCP fixes, a value its catalog entry sets, one the catalog entry binds to a
// Secret, and a required one the vMCP prohibits.
func fixedAndCatalogComponent(id string) types.VMCPComponent {
	component := readinessComponent(id)
	component.CatalogEntry.Manifest.Config = []types.MCPConfig{
		{Key: "REGION", Required: true},
		{Key: "VERBOSE"},
		{Key: "ADMIN_TOKEN", Required: true},
		{Key: "CATALOG_LITERAL", Required: true, Static: true, Value: "catalog"},
		{Key: "CATALOG_SECRET", Required: true, SecretBinding: &types.MCPSecretBinding{Name: "secret", Key: "key"}},
		{Key: "BLOCKED", Required: true},
	}
	component.Configuration = []types.VMCPConfigurationPolicy{
		{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "VERBOSE", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "ADMIN_TOKEN", Policy: types.VMCPConfigurationPolicyFixed},
		{Key: "CATALOG_LITERAL", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "CATALOG_SECRET", Policy: types.VMCPConfigurationPolicyUserAllowed},
		{Key: "BLOCKED", Policy: types.VMCPConfigurationPolicyProhibited},
	}
	return component
}

// hostnameComponent returns a remote component whose URL each connection enters, on a required hostname, unless the
// vMCP fixes it.
func hostnameComponent(id string) types.VMCPComponent {
	component := readinessComponent(id)
	component.CatalogEntry.Manifest.Runtime = types.RuntimeRemote
	component.CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{Hostname: "tenant.example.com"}
	return component
}

// fixedURL returns component with its server URL (__url) fixed by the vMCP.
func fixedURL(component types.VMCPComponent) types.VMCPComponent {
	component.Configuration = append(component.Configuration, types.VMCPConfigurationPolicy{Key: "__url", Policy: types.VMCPConfigurationPolicyFixed})
	return component
}

// everyUserProfile returns a profile that names every user ({selector, "*"}), and so no agent.
func everyUserProfile(id string) types.VMCPProfile {
	return types.VMCPProfile{
		ID:          id,
		Name:        id,
		Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	}
}

// agentNamingProfile returns a profile that names the agent agentID and grants every component.
func agentNamingProfile(id, agentID string) types.VMCPProfile {
	return types.VMCPProfile{
		ID:          id,
		Name:        id,
		Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: agentID}},
		Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
	}
}

// TestAgentMissingConfiguration is the table of what an agent cannot be given: a required value a user would enter,
// and a server URL the vMCP does not fix. Optional values, values the vMCP fixes, values the catalog entry supplies and
// prohibited values never block an agent.
func TestAgentMissingConfiguration(t *testing.T) {
	for _, tt := range []struct {
		name      string
		component types.VMCPComponent
		want      []string
	}{
		{
			name:      "every kind of value",
			component: fixedAndCatalogComponent("api"),
			want:      []string{"REGION"},
		},
		{
			name: "only an optional value a user would enter",
			component: types.VMCPComponent{
				ID:            "optional",
				CatalogEntry:  types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Config: []types.MCPConfig{{Key: "VERBOSE"}}}},
				Configuration: []types.VMCPConfigurationPolicy{{Key: "VERBOSE", Policy: types.VMCPConfigurationPolicyUserAllowed}},
			},
		},
		{
			name:      "nothing to configure",
			component: readinessComponent("plain"),
		},
		{
			name:      "a server URL that the vMCP does not fix",
			component: hostnameComponent("remote"),
			want:      []string{"__url"},
		},
		{
			name:      "a server URL that the vMCP fixes",
			component: fixedURL(hostnameComponent("remote")),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vmcp := readinessVMCP([]types.VMCPComponent{tt.component})
			components := ComponentsForInstance(vmcp, v1.VMCPInstance{})
			require.Equal(t, tt.want, AgentMissingConfiguration(components[0]))
		})
	}
}
