package vmcp

import (
	"context"
	"net/http"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// profileTargets resolves the URL that the configuration of a component reaches for an agent: the URL its explicit
// profile configures, else the one its wildcard profile does, as a required value resolves. It counts its calls.
type profileTargets struct {
	urls  map[string]string
	calls int
}

func (p *profileTargets) ComponentTarget(_ context.Context, _ v1.VMCP, selection AgentSelection, component types.VMCPComponent) (ComponentTarget, error) {
	p.calls++
	selection = selection.ForComponent(component.ID)
	for _, profile := range []*types.VMCPProfile{selection.Explicit, selection.Wildcard} {
		if profile != nil && p.urls[profile.ID] != "" {
			return ComponentTarget{URL: p.urls[profile.ID], ProfileIDs: []string{profile.ID}}, nil
		}
	}
	return ComponentTarget{Missing: []string{"__url"}}, nil
}

// recordProbe records in vmcp's status what a probe of component at url found, as the vMCP controller does.
func recordProbe(vmcp *v1.VMCP, component types.VMCPComponent, url string, required v1.VMCPComponentOAuthRequirement) {
	if vmcp.Status.ComponentOAuth == nil {
		vmcp.Status.ComponentOAuth = map[string]v1.VMCPComponentOAuth{}
	}
	record := vmcp.Status.ComponentOAuth[component.ID]
	record.DefinitionHash = ProfileOAuthGrantDefinitionHash(component)
	record.Probes = append(record.Probes, v1.VMCPComponentOAuthProbe{
		URLDigest: ComponentOAuthURLDigest(vmcp.Name, component.ID, url),
		Required:  required,
		CheckedAt: metav1.Now(),
	})
	vmcp.Status.ComponentOAuth[component.ID] = record
}

// TestProbedOAuthRequirement checks what is read of a probe: only the result for the URL asked about, for the
// component's current definition, and only for a component that is probed at all.
func TestProbedOAuthRequirement(t *testing.T) {
	vmcp := grantTestVMCP()
	dynamic := oauthComponent("dynamic")
	url := dynamic.CatalogEntry.Manifest.RemoteConfig.FixedURL
	recordProbe(&vmcp, dynamic, url, v1.VMCPComponentOAuthRequired)

	tunneled := oauthComponent("dynamic")
	tunneled.CatalogEntry.Manifest.RemoteConfig.TunnelName = "tunnel"
	withHeader := oauthComponent("dynamic")
	withHeader.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "Authorization", Usage: types.Header}}
	static := staticOAuthComponent("dynamic")
	static.CatalogEntry.Manifest.RemoteConfig.FixedURL = url
	other := vmcp.DeepCopy()
	other.Name = "vmcp1other"

	for _, tt := range []struct {
		name      string
		vmcp      v1.VMCP
		component types.VMCPComponent
		url       string
		want      v1.VMCPComponentOAuthRequirement
	}{
		{
			name:      "the URL probed",
			vmcp:      vmcp,
			component: dynamic,
			url:       url,
			want:      v1.VMCPComponentOAuthRequired,
		},
		{
			name:      "a URL not probed",
			vmcp:      vmcp,
			component: dynamic,
			url:       "https://elsewhere.example.com/mcp",
			want:      v1.VMCPComponentOAuthUnknown,
		},
		{
			name:      "no URL",
			vmcp:      vmcp,
			component: dynamic,
			want:      v1.VMCPComponentOAuthUnknown,
		},
		{
			name:      "a changed definition",
			vmcp:      vmcp,
			component: tunneled,
			url:       url,
			want:      v1.VMCPComponentOAuthUnknown,
		},
		{
			// The probe sends no configured header, so it says nothing of a connection that sends one.
			name:      "a component that takes headers",
			vmcp:      vmcp,
			component: withHeader,
			url:       url,
			want:      v1.VMCPComponentOAuthUnknown,
		},
		{
			name:      "a component with static OAuth",
			vmcp:      vmcp,
			component: static,
			url:       url,
			want:      v1.VMCPComponentOAuthUnknown,
		},
		{
			name:      "another vMCP's digest",
			vmcp:      *other,
			component: dynamic,
			url:       url,
			want:      v1.VMCPComponentOAuthUnknown,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ProbedOAuthRequirement(tt.vmcp, tt.component, tt.url))
		})
	}
}

// TestAgentReadinessUsesProbedOAuthRequirement checks that a component without static OAuth, which no profile holds a
// grant for, keeps an agent without a grant from connecting once the vMCP controller's probe found that it asks for
// OAuth, and only then: a probe that failed, one that found no OAuth, or none at all, never blocks the agent. Profile
// status reports the same requirement.
func TestAgentReadinessUsesProbedOAuthRequirement(t *testing.T) {
	dynamic := oauthComponent("dynamic")
	url := dynamic.CatalogEntry.Manifest.RemoteConfig.FixedURL
	for _, tt := range []struct {
		name string
		// recorded is what the probe found, if it ran.
		recorded v1.VMCPComponentOAuthRequirement
		// wildcardGrant stores a grant of the wildcard profile for the component.
		wildcardGrant bool
		want7, want8  []ReadinessReason
		wantRequired  bool
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
			want7:        []ReadinessReason{{ComponentID: "dynamic", Code: types.AgentReadinessOAuthRequired, ProfileID: grantExplicit}},
			want8:        []ReadinessReason{{ComponentID: "dynamic", Code: types.AgentReadinessOAuthRequired, ProfileID: grantWildcard}},
			wantRequired: true,
		},
		{
			name:          "asks for OAuth, and the wildcard profile holds a grant",
			recorded:      v1.VMCPComponentOAuthRequired,
			wildcardGrant: true,
			wantRequired:  true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vmcp := grantTestVMCP()
			vmcp.Spec.Manifest.Components = []types.VMCPComponent{dynamic}
			if tt.recorded != "" {
				recordProbe(&vmcp, dynamic, url, tt.recorded)
			}
			grants := newMemoryGrants()
			if tt.wildcardGrant {
				grants.put(grantOwner(grantWildcard), grantMCPID("dynamic"), "grant", url)
			}
			store := NewProfileOAuthGrantStore(grants)
			for agent, want := range map[uint][]ReadinessReason{7: tt.want7, 8: tt.want8} {
				readiness, err := readinessWith(t, store, vmcp, agentInfo(agent, nil))
				require.NoError(t, err)
				require.Equal(t, want, readiness.Reasons, "agent %d", agent)
			}
			for _, profileID := range []string{grantExplicit, grantWildcard} {
				status, err := store.ProfileOAuthStatus(t.Context(), vmcp, profileID)
				require.NoError(t, err)
				require.Equal(t, tt.wantRequired, status["dynamic"].Required, profileID)
			}
		})
	}
}

// TestAgentReadinessUsesTheProbeOfTheURLTheAgentReaches checks a component whose URL comes from configuration: each
// profile's agents are held to the probe of the URL that their configuration resolves to, and the URL is only resolved,
// which reads configuration, when some probe of the component found that it asks for OAuth.
func TestAgentReadinessUsesTheProbeOfTheURLTheAgentReaches(t *testing.T) {
	remote := hostnameComponent("remote")
	explicitURL, wildcardURL := "https://tenant.example.com/explicit", "https://tenant.example.com/wildcard"
	vmcp := grantTestVMCP()
	vmcp.Spec.Manifest.Components = []types.VMCPComponent{remote}
	vmcp.Status.ProfileConfigurationRevisions = profileRevisions("remote", map[string][]string{grantExplicit: {"__url"}, grantWildcard: {"__url"}})
	components := ComponentsForInstance(vmcp, v1.VMCPInstance{})
	targets := &profileTargets{urls: map[string]string{grantExplicit: explicitURL, grantWildcard: wildcardURL}}
	store := NewProfileOAuthGrantStore(newMemoryGrants())
	store.SetComponentTargets(targets)
	reasons := func(agent uint) []ReadinessReason {
		t.Helper()
		readiness, err := readinessWith(t, store, vmcp, agentInfo(agent, nil))
		require.NoError(t, err)
		return readiness.Reasons
	}
	required := func(profileID string) bool {
		t.Helper()
		status, err := store.ProfileOAuthStatus(t.Context(), vmcp, profileID)
		require.NoError(t, err)
		return status["remote"].Required
	}

	// No probe found OAuth, so no URL is resolved to look one up.
	recordProbe(&vmcp, components[0], explicitURL, v1.VMCPComponentOAuthNotRequired)
	recordProbe(&vmcp, components[0], wildcardURL, v1.VMCPComponentOAuthUnknown)
	require.Empty(t, reasons(7))
	require.Empty(t, reasons(8))
	require.False(t, required(grantExplicit))
	require.False(t, required(grantWildcard))
	require.Zero(t, targets.calls)

	// The wildcard profile's URL asks for OAuth; the explicit profile's does not.
	vmcp.Status.ComponentOAuth = nil
	recordProbe(&vmcp, components[0], explicitURL, v1.VMCPComponentOAuthNotRequired)
	recordProbe(&vmcp, components[0], wildcardURL, v1.VMCPComponentOAuthRequired)
	require.Empty(t, reasons(7))
	require.Equal(t, []ReadinessReason{{ComponentID: "remote", Code: types.AgentReadinessOAuthRequired, ProfileID: grantWildcard}}, reasons(8))
	require.False(t, required(grantExplicit))
	require.True(t, required(grantWildcard))
	require.NotZero(t, targets.calls)
}

// TestProfileStatusCoversOnlyGrantedComponents checks that a profile's setup status lists the components it grants,
// all of them with allowAllComponents and those named in allowedComponents otherwise, and no other, for configuration
// and OAuth alike.
func TestProfileStatusCoversOnlyGrantedComponents(t *testing.T) {
	for _, tt := range []struct {
		name        string
		permissions types.VMCPProfilePermissions
		want        []string
	}{
		{
			name:        "allowAllComponents",
			permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
			want:        []string{"static", "dynamic", "local"},
		},
		{
			name:        "allowedComponents",
			permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"dynamic": {}, "local": {AllowedTools: []string{"tool"}}}},
			want:        []string{"dynamic", "local"},
		},
		{
			// An entry that grants no tools still grants the component.
			name:        "a component granted no tools",
			permissions: types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"static": {AllowedTools: []string{}}}},
			want:        []string{"static"},
		},
		{
			name: "no component",
			want: []string{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			profile := explicitAgentProfile(grantExplicit, "7")
			profile.Permissions = tt.permissions
			vmcp := grantTestVMCP(profile, everyAgentProfile(grantWildcard, types.SubjectTypeAgent))
			grants := newMemoryGrants()
			// Grants the profile kept for components it no longer grants are not reported.
			for _, componentID := range []string{"static", "dynamic"} {
				grants.put(grantOwner(grantExplicit), grantMCPID(componentID), "grant-"+componentID, "https://"+componentID+".example.com/mcp")
			}
			oauth, err := NewProfileOAuthGrantStore(grants).ProfileOAuthStatus(t.Context(), vmcp, grantExplicit)
			require.NoError(t, err)
			status := ProfileStatus(vmcp, profile)
			AddOAuthStatus(&status, oauth)

			listed := []string{}
			for _, component := range status.Components {
				listed = append(listed, component.ComponentID)
			}
			require.Equal(t, tt.want, listed)
			for componentID := range oauth {
				require.Contains(t, tt.want, componentID)
			}
		})
	}
}

// TestProfileSetupRefusesComponentsTheProfileDoesNotGrant checks that configuration and the start of an OAuth grant
// are refused with a 400 that names the component, for a component the profile does not grant, and that a reset of the
// whole profile is not.
func TestProfileSetupRefusesComponentsTheProfileDoesNotGrant(t *testing.T) {
	client, vmcp := newProfileConfigurationTestVMCP(t)
	key := kclient.ObjectKeyFromObject(vmcp)
	vmcp.Spec.Manifest.Profiles[0].Permissions = types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"api": {}}}
	require.NoError(t, client.Update(t.Context(), vmcp))
	store := NewProfileConfigurationStore(newMemoryCredentials())

	_, err := store.Update(t.Context(), client, key, profileExplicit, map[string]map[string]*string{
		"api":    {"REGION": new("us-east")},
		"remote": {"__url": new("https://tenant.example.com/mcp")},
	})
	requireHTTPError(t, err, http.StatusBadRequest, `does not grant component "remote"`)
	_, err = store.Update(t.Context(), client, key, profileExplicit, map[string]map[string]*string{"remote": {"__url": nil}})
	requireHTTPError(t, err, http.StatusBadRequest, `does not grant component "remote"`)
	_, err = store.Update(t.Context(), client, key, profileExplicit, map[string]map[string]*string{"missing": {"KEY": new("value")}})
	requireHTTPError(t, err, http.StatusBadRequest, `unknown component ID "missing"`)

	updated, err := store.Update(t.Context(), client, key, profileExplicit, map[string]map[string]*string{"api": {"REGION": new("us-east")}})
	require.NoError(t, err)
	require.Equal(t, []string{ConfigurationKey("api", "REGION")}, updated.Status.ProfileConfigurationRevisions[profileExplicit].Keys)
	_, cleared, err := store.Reset(t.Context(), client, key, profileExplicit)
	require.NoError(t, err)
	require.True(t, cleared)

	stored := getVMCP(t, client, vmcp)
	_, _, err = ProfileOAuthGrantTarget(stored, profileExplicit, "remote")
	requireHTTPError(t, err, http.StatusBadRequest, `does not grant component "remote"`)
	profile, component, err := ProfileOAuthGrantTarget(stored, profileAgents, "remote")
	require.NoError(t, err)
	require.Equal(t, profileAgents, profile.ID)
	require.Equal(t, "remote", component.ID)
}

// TestAgentResolvesComponentsItsExplicitProfileDoesNotGrantFromTheWildcard checks that an agent whose explicit profile
// does not grant a component, which the wildcard profile grants, takes all of the component's setup from the wildcard
// profile: its required and optional values and its OAuth grant, even when the explicit profile kept values and a grant
// for the component from when it granted it, and that what is missing is reported for the wildcard profile to fix.
func TestAgentResolvesComponentsItsExplicitProfileDoesNotGrantFromTheWildcard(t *testing.T) {
	api, dynamic := configuredComponent("api"), oauthComponent("dynamic")
	explicit := explicitAgentProfile(grantExplicit, "7")
	explicit.Permissions = types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"local": {}}}
	vmcp := grantTestVMCP(explicit, everyAgentProfile(grantWildcard, types.SubjectTypeAgent))
	vmcp.Spec.Manifest.Components = []types.VMCPComponent{api, dynamic, readinessComponent("local")}
	vmcp.Status.ProfileConfigurationRevisions = profileRevisions("api", map[string][]string{
		grantExplicit: {"REGION", "VERBOSE"},
		grantWildcard: {"REGION", "VERBOSE"},
	})
	grants := newMemoryGrants()
	grants.put(grantOwner(grantExplicit), grantMCPID("dynamic"), "kept", dynamic.CatalogEntry.Manifest.RemoteConfig.FixedURL)
	grants.put(grantOwner(grantWildcard), grantMCPID("dynamic"), "wildcard", dynamic.CatalogEntry.Manifest.RemoteConfig.FixedURL)
	store := NewProfileOAuthGrantStore(grants)
	agent := agentInfo(7, nil)

	configuration := ResolveAgentConfiguration(vmcp, v1.VMCPInstance{}, agent)
	require.Len(t, configuration.Components, 3)
	require.Equal(t, []ConfigurationSource{
		{ComponentID: "api", Field: "REGION", ProfileID: grantWildcard, Fallback: true, Revision: "configuration-" + grantWildcard},
		{ComponentID: "api", Field: "VERBOSE", ProfileID: grantWildcard, Fallback: true, Revision: "configuration-" + grantWildcard},
	}, configuration.Components[0].Sources)
	require.Empty(t, configuration.Components[0].Missing)
	require.Equal(t, map[string]string{grantWildcard: "configuration-" + grantWildcard}, configuration.Revisions)

	selected, err := store.SelectAgentGrants(t.Context(), vmcp, SelectAgentProfiles(agent, vmcp.Spec.Manifest), vmcp.Spec.Manifest.Components)
	require.NoError(t, err)
	require.Equal(t, grantWildcard, selected[1].ProfileID)
	require.True(t, selected[1].Fallback)
	require.Equal(t, grantOwner(grantWildcard), selected[1].Owner)

	readiness, err := readinessWith(t, store, vmcp, agent)
	require.NoError(t, err)
	require.True(t, readiness.Ready(), "%v", readiness.Reasons)

	// What the component lacks is fixed in the wildcard profile, the one that can set it up.
	vmcp.Status.ProfileConfigurationRevisions = profileRevisions("api", map[string][]string{grantExplicit: {"REGION", "VERBOSE"}})
	require.NoError(t, grants.DeleteMCPOAuthGrants(t.Context(), grantOwner(grantWildcard), grantMCPID("dynamic"), []string{"wildcard"}))
	readiness, err = readinessWith(t, store, vmcp, agent)
	require.NoError(t, err)
	require.Equal(t, []ReadinessReason{
		{ComponentID: "api", Code: types.AgentReadinessConfigurationRequired, Field: "REGION", ProfileID: grantWildcard},
		{ComponentID: "dynamic", Code: types.AgentReadinessOAuthRequired, ProfileID: grantWildcard},
	}, readiness.Reasons)

	// Once the explicit profile grants the component again, the setup it kept is its own again.
	vmcp.Spec.Manifest.Profiles[0].Permissions = types.VMCPProfilePermissions{AllowAllComponents: true}
	configuration = ResolveAgentConfiguration(vmcp, v1.VMCPInstance{}, agent)
	require.Equal(t, grantExplicit, configuration.Components[0].Sources[0].ProfileID)
	readiness, err = readinessWith(t, store, vmcp, agent)
	require.NoError(t, err)
	require.True(t, readiness.Ready(), "%v", readiness.Reasons)
}
