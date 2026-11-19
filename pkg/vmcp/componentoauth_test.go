package vmcp

import (
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// recordProbe records in vmcp's status what a probe of component found, as the vMCP controller does.
func recordProbe(vmcp *v1.VMCP, component types.VMCPComponent, required v1.VMCPComponentOAuthRequirement) {
	if vmcp.Status.ComponentOAuth == nil {
		vmcp.Status.ComponentOAuth = map[string]v1.VMCPComponentOAuth{}
	}
	vmcp.Status.ComponentOAuth[component.ID] = v1.VMCPComponentOAuth{
		TargetHash: ComponentOAuthTargetHash(*vmcp, component),
		Required:   required,
		CheckedAt:  metav1.Now(),
	}
}

// TestProbedOAuthRequirement checks what is read of a probe: only the result for the component's current target, its
// definition and the values the vMCP fixes for it, and only for a component that is probed at all.
func TestProbedOAuthRequirement(t *testing.T) {
	vmcp := grantTestVMCP()
	dynamic := oauthComponent("dynamic")
	recordProbe(&vmcp, dynamic, v1.VMCPComponentOAuthRequired)

	tunneled := oauthComponent("dynamic")
	tunneled.CatalogEntry.Manifest.RemoteConfig.TunnelName = "tunnel"
	withHeader := oauthComponent("dynamic")
	withHeader.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "Authorization", Usage: types.Header}}
	static := staticOAuthComponent("dynamic")
	refixed := vmcp.DeepCopy()
	refixed.Spec.ComponentStaticConfigurationHashes = map[string]string{"dynamic": "changed"}

	for _, tt := range []struct {
		name      string
		vmcp      v1.VMCP
		component types.VMCPComponent
		want      v1.VMCPComponentOAuthRequirement
	}{
		{
			name:      "the component probed",
			vmcp:      vmcp,
			component: dynamic,
			want:      v1.VMCPComponentOAuthRequired,
		},
		{
			name:      "a component not probed",
			vmcp:      vmcp,
			component: oauthComponent("other"),
			want:      v1.VMCPComponentOAuthUnknown,
		},
		{
			name:      "a changed definition",
			vmcp:      vmcp,
			component: tunneled,
			want:      v1.VMCPComponentOAuthUnknown,
		},
		{
			// The URL may be built from them, so the probe may be of another URL.
			name:      "changed values the vMCP fixes",
			vmcp:      *refixed,
			component: dynamic,
			want:      v1.VMCPComponentOAuthUnknown,
		},
		{
			// The probe sends no configured header, so it says nothing of a connection that sends one.
			name:      "a component that takes headers",
			vmcp:      vmcp,
			component: withHeader,
			want:      v1.VMCPComponentOAuthUnknown,
		},
		{
			name:      "a component with static OAuth",
			vmcp:      vmcp,
			component: static,
			want:      v1.VMCPComponentOAuthUnknown,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ProbedOAuthRequirement(tt.vmcp, tt.component))
		})
	}
}

// TestAgentReadinessUsesProbedOAuthRequirement checks when a component without static OAuth keeps the agents granted
// it from the vMCP: once the vMCP controller's probe found that it asks for OAuth, or once the vMCP holds a grant for
// it, which proves it does (a server that challenges only late), and the vMCP holds no working grant for it. A probe
// that failed, one that found no OAuth, or none at all, never blocks an agent. Every agent granted the component gets
// the same answer, and so does the vMCP's agent setup.
func TestAgentReadinessUsesProbedOAuthRequirement(t *testing.T) {
	dynamic := oauthComponent("dynamic")
	url := dynamic.CatalogEntry.Manifest.RemoteConfig.FixedURL
	required := []ReadinessReason{{ComponentID: "dynamic", Code: types.AgentReadinessOAuthRequired}}
	invalid := []ReadinessReason{{ComponentID: "dynamic", Code: types.AgentReadinessOAuthInvalid}}
	for _, tt := range []struct {
		name string
		// recorded is what the probe found, if it ran.
		recorded v1.VMCPComponentOAuthRequirement
		// grant stores the vMCP's grant for the component, and invalid marks it invalid.
		grant        bool
		invalid      bool
		want         []ReadinessReason
		wantRequired bool
	}{
		{
			name: "not probed",
		},
		{
			name:     "probe failed",
			recorded: v1.VMCPComponentOAuthUnknown,
		},
		{
			name:     "no OAuth",
			recorded: v1.VMCPComponentOAuthNotRequired,
		},
		{
			name:         "asks for OAuth",
			recorded:     v1.VMCPComponentOAuthRequired,
			want:         required,
			wantRequired: true,
		},
		{
			name:         "asks for OAuth, and the vMCP holds a grant",
			recorded:     v1.VMCPComponentOAuthRequired,
			grant:        true,
			wantRequired: true,
		},
		{
			// A server that challenges only late is not found by every probe; the grant shows it asks for OAuth.
			name:         "not probed, and the vMCP holds a grant",
			grant:        true,
			wantRequired: true,
		},
		{
			name:         "not probed, and the vMCP holds a broken grant",
			grant:        true,
			invalid:      true,
			want:         invalid,
			wantRequired: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vmcp := grantTestVMCP()
			vmcp.Spec.Manifest.Components = []types.VMCPComponent{dynamic}
			if tt.recorded != "" {
				recordProbe(&vmcp, dynamic, tt.recorded)
			}
			grants := newMemoryGrants()
			if tt.grant {
				grants.put(grantOwner(), grantMCPID("dynamic"), "grant", url)
			}
			if tt.invalid {
				grants.invalidate(grantOwner(), grantMCPID("dynamic"))
			}
			store := NewAgentOAuthGrantStore(grants)
			for _, agent := range []uint{7, 9} {
				readiness, err := readinessWith(t, store, vmcp, agentInfo(agent, nil))
				require.NoError(t, err)
				require.Equal(t, tt.want, readiness.Reasons, "agent %d", agent)
			}
			setup, err := AgentSetup(t.Context(), store, vmcp)
			require.NoError(t, err)
			require.Len(t, setup.Components, 1)
			require.Equal(t, tt.wantRequired, setup.Components[0].OAuth.Required)
			require.Equal(t, len(tt.want) == 0, setup.Components[0].Ready)
		})
	}
}

// TestAgentSetupCoversOnlyAgentGrantedComponents checks that the vMCP's agent setup lists the components that profiles
// naming agents grant, all of them with allowAllComponents and those named in allowedComponents otherwise, the union
// over every such profile, and no component that only a profile for every user grants, even one the vMCP holds a grant
// for.
func TestAgentSetupCoversOnlyAgentGrantedComponents(t *testing.T) {
	only := func(componentIDs ...string) types.VMCPProfilePermissions {
		allowed := map[string]types.VMCPComponentSet{}
		for _, id := range componentIDs {
			allowed[id] = types.VMCPComponentSet{}
		}
		return types.VMCPProfilePermissions{AllowedComponents: allowed}
	}
	for _, tt := range []struct {
		name string
		// permissions7 and permissions9 are what the profiles of agents 7 and 9 grant.
		permissions7 types.VMCPProfilePermissions
		permissions9 types.VMCPProfilePermissions
		want         []string
	}{
		{
			name:         "allowAllComponents",
			permissions7: types.VMCPProfilePermissions{AllowAllComponents: true},
			want:         []string{"static", "dynamic", "local"},
		},
		{
			name:         "allowedComponents",
			permissions7: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"dynamic": {}, "local": {AllowedTools: []string{"tool"}}}},
			want:         []string{"dynamic", "local"},
		},
		{
			// An entry that grants no tools still grants the component.
			name:         "a component granted no tools",
			permissions7: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"static": {AllowedTools: []string{}}}},
			want:         []string{"static"},
		},
		{
			name:         "the union of two agents' profiles",
			permissions7: only("local"),
			permissions9: only("static", "local"),
			want:         []string{"static", "local"},
		},
		{
			name: "no component",
			want: []string{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			seven, nine := agentNamingProfile(grantAgent7, "7"), agentNamingProfile(grantAgent9, "9")
			seven.Permissions, nine.Permissions = tt.permissions7, tt.permissions9
			vmcp := grantTestVMCP(seven, nine, everyUserProfile(grantUsers))
			grants := newMemoryGrants()
			// The vMCP keeps its grants for components that no agent is granted any more, and they are not reported.
			for _, componentID := range []string{"static", "dynamic"} {
				grants.put(grantOwner(), grantMCPID(componentID), "grant-"+componentID, "https://"+componentID+".example.com/mcp")
			}
			setup, err := AgentSetup(t.Context(), NewAgentOAuthGrantStore(grants), vmcp)
			require.NoError(t, err)

			listed := []string{}
			for _, component := range setup.Components {
				listed = append(listed, component.ComponentID)
			}
			require.Equal(t, tt.want, listed)
			granted := []string{}
			for _, component := range AgentGrantedComponents(vmcp) {
				granted = append(granted, component.ID)
			}
			require.Equal(t, tt.want, granted)
		})
	}
}

// TestAgentUsesTheVMCPsGrantOnlyForComponentsItIsGranted checks that the vMCP's grant for a component serves only the
// agents that a profile naming them grants the component: while the agent's profile does not grant it, the component
// is not enabled for the agent, no grant is selected for it, and a profile for every user that grants it supplies the
// agent nothing. Once a profile naming the agent grants the component, the vMCP's grant, kept all along, serves it.
func TestAgentUsesTheVMCPsGrantOnlyForComponentsItIsGranted(t *testing.T) {
	dynamic := oauthComponent("dynamic")
	vmcp := grantTestVMCP(agentNamingProfileFor(grantAgent7, "7", "local"), everyUserProfile(grantUsers))
	vmcp.Spec.Manifest.Components = []types.VMCPComponent{dynamic, readinessComponent("local")}
	grants := newMemoryGrants()
	grants.put(grantOwner(), grantMCPID("dynamic"), "kept", dynamic.CatalogEntry.Manifest.RemoteConfig.FixedURL)
	store := NewAgentOAuthGrantStore(grants)
	agent := agentInfo(7, nil)
	grantFor := func(report AgentReadinessReport, componentID string) (ComponentOAuthGrant, bool) {
		index := slices.IndexFunc(report.Grants, func(grant ComponentOAuthGrant) bool { return grant.ComponentID == componentID })
		if index < 0 {
			return ComponentOAuthGrant{}, false
		}
		return report.Grants[index], true
	}

	report, err := AgentReadiness(t.Context(), store, vmcp, nil, agent)
	require.NoError(t, err)
	require.True(t, report.Readiness.Ready(), "%v", report.Readiness.Reasons)
	_, selected := grantFor(report, "dynamic")
	require.False(t, selected, "the vMCP's grant was selected for a component the agent is not granted")

	// Granted the component by a second profile, the agent uses the vMCP's grant.
	vmcp.Spec.Manifest.Profiles = append(vmcp.Spec.Manifest.Profiles, agentNamingProfileFor("vp1dynamic", "7", "dynamic"))
	report, err = AgentReadiness(t.Context(), store, vmcp, nil, agent)
	require.NoError(t, err)
	require.True(t, report.Readiness.Ready(), "%v", report.Readiness.Reasons)
	grant, selected := grantFor(report, "dynamic")
	require.True(t, selected)
	require.True(t, grant.Usable())
	require.Equal(t, grantOwner(), grant.Owner)
	require.Equal(t, grantMCPID("dynamic"), grant.MCPID)
}
