package vmcp

import (
	"context"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentdelegation"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	pairVMCP = "vmcp1pair"
	// pairOwner is agent 7 acting on behalf of user 9.
	pairOwner = "agent:7/user:9"
)

// recordingDelegatedGateway records which pairs DeleteDelegatedOwnerMCPOAuthState would delete the state of.
type recordingDelegatedGateway struct {
	match func(principal.DelegatedOwner) bool
}

// pairComponents are a vMCP's components, each needing something different from a user:
//   - fixed: a local server whose only value the vMCP fixes.
//   - keyed: a local server that takes a value the user enters.
//   - granted: a remote server with a fixed URL, which the vMCP's preconfigured OAuth covers.
//   - granted-headers: the same, with a header the user enters.
//   - oauth: a remote server with a fixed URL and no grant, which asks for OAuth.
//   - open: a remote server with a fixed URL and no grant, not known to ask for OAuth.
func pairComponents() []types.VMCPComponent {
	local := func(id string, policy types.VMCPConfigurationPolicyType) types.VMCPComponent {
		return types.VMCPComponent{
			ID:            id,
			Name:          id,
			Configuration: []types.VMCPConfigurationPolicy{{Key: "TOKEN", Policy: policy}},
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:   types.RuntimeNPX,
				NPXConfig: &types.NPXRuntimeConfig{Package: "@example/" + id},
				Config:    []types.MCPConfig{{Key: "TOKEN", Name: "Token", Usage: types.Env, Required: true, Sensitive: true}},
			}},
		}
	}
	remote := func(id string, header bool) types.VMCPComponent {
		component := types.VMCPComponent{
			ID:   id,
			Name: id,
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteCatalogConfig{FixedURL: "https://" + id + ".example.com/mcp"},
			}},
		}
		if header {
			component.Configuration = []types.VMCPConfigurationPolicy{{Key: "X-Team", Policy: types.VMCPConfigurationPolicyUserAllowed}}
			component.CatalogEntry.Manifest.Config = []types.MCPConfig{{Key: "X-Team", Usage: types.Header}}
		}
		return component
	}
	return []types.VMCPComponent{
		local("fixed", types.VMCPConfigurationPolicyFixed),
		local("keyed", types.VMCPConfigurationPolicyUserAllowed),
		remote("granted", false),
		remote("granted-headers", true),
		remote("oauth", false),
		remote("open", false),
	}
}

// pairTestVMCP returns a shared vMCP with pairComponents, a profile that names agent 7 and grants all of them, and the
// controller's probe finding that "oauth" asks for OAuth.
func pairTestVMCP() v1.VMCP {
	vmcp := v1.VMCP{
		Name:      pairVMCP,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			Components: pairComponents(),
			Profiles: []types.VMCPProfile{{
				ID:          "vp1agent",
				Subjects:    []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}},
				Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
			}},
		}},
	}
	oauth := vmcp.Spec.Manifest.Components[4]
	vmcp.Status.ComponentOAuth = map[string]v1.VMCPComponentOAuth{
		oauth.ID: {TargetHash: ComponentOAuthTargetHash(vmcp, oauth), Required: v1.VMCPComponentOAuthRequired},
	}
	return vmcp
}

// pairTestGrants returns the store of the vMCP's preconfigured OAuth grants, holding one for "granted" and "granted-headers".
func pairTestGrants() (*PreconfiguredOAuthStore, *memoryGrants) {
	stored := newMemoryGrants()
	owner := principal.PreconfiguredOAuthOwner(pairVMCP)
	for _, id := range []string{"granted", "granted-headers"} {
		stored.put(owner, PreconfiguredOAuthMCPID(pairVMCP, id), "grant-"+id, "https://"+id+".example.com/mcp")
	}
	return NewPreconfiguredOAuthStore(stored), stored
}

func pairInstance(name string, missing ...string) *v1.VMCPInstance {
	return &v1.VMCPInstance{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec:      v1.VMCPInstanceSpec{UserID: pairOwner, Manifest: types.VMCPInstanceManifest{VMCPID: pairVMCP}},
		Status:    v1.VMCPInstanceStatus{MissingRequiredConfiguration: missing},
	}
}

func componentIDs(components []types.VMCPComponent) []string {
	ids := make([]string, 0, len(components))
	for _, component := range components {
		ids = append(ids, component.ID)
	}
	return ids
}

// TestWhereComponentsRunOnBehalfOfAUser checks which components an agent acting on behalf of a user reaches through
// the pair's setup: those that take a value from the user, and remote ones without preconfigured OAuth. A component that
// the vMCP fixes, and one whose OAuth the vMCP's preconfigured OAuth covers, run on the agent's own instance; a component with
// a grant that takes a header from the user runs on the pair's, with the grant.
func TestWhereComponentsRunOnBehalfOfAUser(t *testing.T) {
	vmcp := pairTestVMCP()
	grants, _ := pairTestGrants()

	selection, err := SelectDelegated(t.Context(), grants, vmcp, nil, DelegatedProfiles(7, vmcp))
	require.NoError(t, err)
	require.Equal(t, []string{"fixed", "keyed", "granted", "granted-headers", "oauth", "open"}, componentIDs(selection.Components))
	require.Equal(t, []bool{false, true, false, true, true, true}, selection.UsesPair)
	require.True(t, selection.NeedsPair())
	held := make([]bool, len(selection.Grants))
	for i, grant := range selection.Grants {
		held[i] = grant.Held()
	}
	require.Equal(t, []bool{false, false, true, true, false, false}, held, "the agent's grant is used wherever the component runs")

	// The pair's instance carries everything that may need the user, whatever the grants and the profiles: so that a
	// grant being revoked, or a profile changing, never removes what the user set up.
	require.Equal(t, []string{"keyed", "granted", "granted-headers", "oauth", "open"}, componentIDs(PairInstanceComponents(pairComponents())))

	// A selection of only what needs nothing from the user needs no pair instance.
	onlyFixed := vmcp.DeepCopy()
	onlyFixed.Spec.Manifest.Profiles[0].Permissions = types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{"fixed": {}, "granted": {}}}
	selection, err = SelectDelegated(t.Context(), grants, *onlyFixed, nil, DelegatedProfiles(7, *onlyFixed))
	require.NoError(t, err)
	require.False(t, selection.NeedsPair())
	require.True(t, selection.Readiness(PairSetup{}).Ready())
}

// TestDelegatedReadiness checks the agent's readiness on behalf of a user: fixed values always apply, the vMCP's agent
// grant beats the user's account, a required value the user enters must be set through the agent, and an account is
// needed only where the vMCP holds no grant for a component known to ask for OAuth. What the user can fix is reported
// as needing their approval, and a broken grant as the administrator's.
func TestDelegatedReadiness(t *testing.T) {
	vmcp := pairTestVMCP()
	grants, stored := pairTestGrants()
	selection, err := SelectDelegated(t.Context(), grants, vmcp, nil, DelegatedProfiles(7, vmcp))
	require.NoError(t, err)

	needsValue := ReadinessReason{ComponentID: "keyed", Code: types.AgentReadinessConfigurationRequired, Field: "TOKEN"}
	needsAccount := ReadinessReason{ComponentID: "oauth", Code: types.AgentReadinessOAuthRequired}

	t.Run("without a pair instance", func(t *testing.T) {
		readiness := selection.Readiness(PairSetup{})
		require.Equal(t, []ReadinessReason{needsValue, needsAccount}, readiness.Reasons)
		failure := (&AgentNotReadyError{Readiness: readiness, VMCPID: pairVMCP, OnBehalfOfUser: true}).Failure()
		require.Equal(t, AgentUserApprovalRequiredError, failure.Error)
		require.Equal(t, pairVMCP, failure.VMCPID)
	})
	t.Run("missing values recorded on the pair's instance", func(t *testing.T) {
		readiness := selection.Readiness(PairSetup{Instance: pairInstance("vmcpi1pair", ConfigurationKey("keyed", "TOKEN")), Connected: map[string]bool{"oauth": true}})
		require.Equal(t, []ReadinessReason{needsValue}, readiness.Reasons)
	})
	t.Run("values read from the pair's credential", func(t *testing.T) {
		readiness := selection.Readiness(PairSetup{
			// The instance records an old state; the values read now win.
			Instance:  pairInstance("vmcpi1pair", ConfigurationKey("keyed", "TOKEN")),
			Values:    map[string]string{ConfigurationKey("keyed", "TOKEN"): "secret"},
			Connected: map[string]bool{"oauth": true},
		})
		require.True(t, readiness.Ready(), "%+v", readiness.Reasons)
	})
	t.Run("an account connected for a component the grant covers is never needed", func(t *testing.T) {
		readiness := selection.Readiness(PairSetup{Instance: pairInstance("vmcpi1pair"), Connected: map[string]bool{"oauth": true}})
		require.True(t, readiness.Ready(), "%+v", readiness.Reasons)
	})
	t.Run("a broken grant is the administrator's", func(t *testing.T) {
		stored.invalidate(principal.PreconfiguredOAuthOwner(pairVMCP), PreconfiguredOAuthMCPID(pairVMCP, "granted"))
		selection, err := SelectDelegated(t.Context(), grants, vmcp, nil, DelegatedProfiles(7, vmcp))
		require.NoError(t, err)
		readiness := selection.Readiness(PairSetup{Instance: pairInstance("vmcpi1pair"), Connected: map[string]bool{"oauth": true}})
		require.Equal(t, []ReadinessReason{{ComponentID: "granted", Code: types.AgentReadinessOAuthInvalid}}, readiness.Reasons)
		require.Equal(t, AgentOAuthRequiredError, (&AgentNotReadyError{Readiness: readiness, OnBehalfOfUser: true}).Failure().Error)
	})
}

// TestDelegatedProfilesAgreeWithCallerProfiles checks that the setup a user does through an agent is of the same
// profiles as the agent's connection on behalf of the user: those that name the agent.
func TestDelegatedProfilesAgreeWithCallerProfiles(t *testing.T) {
	vmcp := pairTestVMCP()
	vmcp.Spec.Manifest.Profiles = append(vmcp.Spec.Manifest.Profiles,
		types.VMCPProfile{ID: "vp1other", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}}},
		types.VMCPProfile{ID: "vp1users", Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}},
	)
	want := CallerProfiles(onBehalfOf(t), vmcp)
	require.Len(t, want, 2)
	require.Equal(t, want, DelegatedProfiles(7, vmcp))
	require.Empty(t, DelegatedProfiles(8, vmcp), "another agent")
}

// TestPairConnections checks that a pair's upstream accounts are found by the pair's own connection to each remote
// component, and never by another owner's token for the same connection name.
func TestPairConnections(t *testing.T) {
	grants, stored := pairTestGrants()
	instance := pairInstance("vmcpi1pair")
	components := pairComponents()
	oauth := components[4]
	open := components[5]
	stored.put(pairOwner, PairConnectID(instance.Name, oauth), "pair-oauth", "https://oauth.example.com/mcp")
	// Another pair's, and the user's own, under the same connection names, are not the pair's.
	stored.put("agent:7/user:10", PairConnectID(instance.Name, open), "other-pair", "https://open.example.com/mcp")
	stored.put("9", PairConnectID(instance.Name, open), "user-own", "https://open.example.com/mcp")

	connected, err := grants.PairConnections(t.Context(), instance, components)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"oauth": true}, connected)

	connected, err = grants.PairConnections(t.Context(), nil, components)
	require.NoError(t, err)
	require.Empty(t, connected)

	_, err = grants.PairConnections(t.Context(), &v1.VMCPInstance{Name: "vmcpi1user", Spec: v1.VMCPInstanceSpec{UserID: "9"}}, components)
	require.Error(t, err, "a user's own instance is not a pair's")
}

func (r *recordingDelegatedGateway) DeleteDelegatedOwnerMCPOAuthState(_ context.Context, match func(principal.DelegatedOwner) bool) error {
	r.match = match
	return nil
}

// TestDelegatedSetupCleanup checks that revoking a delegation removes only its pair's setup, and deleting an agent or a
// user removes every pair of theirs, never a user's own or an agent's own instance.
func TestDelegatedSetupCleanup(t *testing.T) {
	instance := func(name, owner string) *v1.VMCPInstance {
		return &v1.VMCPInstance{Name: name, Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: pairVMCP}}}
	}
	objects := []kclient.Object{
		instance("vmcpi1pair", pairOwner),
		instance("vmcpi1pairother", "agent:7/user:10"),
		instance("vmcpi1otheragent", "agent:8/user:9"),
		instance("vmcpi1user", "9"),
		instance("vmcpi1agent", "agent:7"),
	}
	remaining := func(t *testing.T, client kclient.Client) []string {
		t.Helper()
		var list v1.VMCPInstanceList
		require.NoError(t, client.List(t.Context(), &list))
		var names []string
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
		return names
	}
	for _, tt := range []struct {
		name      string
		scope     agentdelegation.Scope
		remaining []string
		deletes   []principal.DelegatedOwner
		keeps     []principal.DelegatedOwner
	}{
		{
			name:      "revoked delegation",
			scope:     agentdelegation.Scope{AgentID: 7, UserID: 9},
			remaining: []string{"vmcpi1agent", "vmcpi1otheragent", "vmcpi1pairother", "vmcpi1user"},
			deletes:   []principal.DelegatedOwner{{AgentID: 7, UserID: 9}},
			keeps:     []principal.DelegatedOwner{{AgentID: 7, UserID: 10}, {AgentID: 8, UserID: 9}},
		},
		{
			name:      "deleted agent",
			scope:     agentdelegation.Scope{AgentID: 7},
			remaining: []string{"vmcpi1agent", "vmcpi1otheragent", "vmcpi1user"},
			deletes:   []principal.DelegatedOwner{{AgentID: 7, UserID: 9}, {AgentID: 7, UserID: 10}},
			keeps:     []principal.DelegatedOwner{{AgentID: 8, UserID: 9}},
		},
		{
			name:      "deleted user",
			scope:     agentdelegation.Scope{UserID: 9},
			remaining: []string{"vmcpi1agent", "vmcpi1pairother", "vmcpi1user"},
			deletes:   []principal.DelegatedOwner{{AgentID: 7, UserID: 9}, {AgentID: 8, UserID: 9}},
			keeps:     []principal.DelegatedOwner{{AgentID: 7, UserID: 10}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(objects...).
				WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.UserID} }).
				Build()
			gateway := &recordingDelegatedGateway{}
			cleanup := NewDelegatedSetupCleanup(client, gateway)
			require.NoError(t, cleanup.CleanupDelegations(t.Context(), tt.scope))
			require.Equal(t, tt.remaining, remaining(t, client))
			for _, pair := range tt.deletes {
				require.True(t, gateway.match(pair), "the upstream tokens of %v are kept", pair)
			}
			for _, pair := range tt.keeps {
				require.False(t, gateway.match(pair), "the upstream tokens of %v are deleted", pair)
			}
			// It can run again.
			require.NoError(t, cleanup.CleanupDelegations(t.Context(), tt.scope))
		})
	}

	require.Error(t, NewDelegatedSetupCleanup(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), &recordingDelegatedGateway{}).CleanupDelegations(t.Context(), agentdelegation.Scope{}))
}

// TestDeletePairInstances checks that deleting a vMCP deletes what users set up for it through agents, and nothing
// else: another vMCP's, or a user's or an agent's own instance, which the vMCP's other cleanup handles.
func TestDeletePairInstances(t *testing.T) {
	instance := func(name, vmcpID, owner string) *v1.VMCPInstance {
		return &v1.VMCPInstance{Name: name, Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{UserID: owner, Manifest: types.VMCPInstanceManifest{VMCPID: vmcpID}}}
	}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(
		instance("vmcpi1pair", pairVMCP, pairOwner),
		instance("vmcpi1pairelsewhere", "vmcp1other", pairOwner),
		instance("vmcpi1user", pairVMCP, "9"),
		instance("vmcpi1agent", pairVMCP, "agent:7"),
	).WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string {
		return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID}
	}).Build()

	require.NoError(t, DeletePairInstances(t.Context(), client, pairTestVMCP()))
	require.NoError(t, DeletePairInstances(t.Context(), client, pairTestVMCP()))
	var list v1.VMCPInstanceList
	require.NoError(t, client.List(t.Context(), &list))
	var names []string
	for _, item := range list.Items {
		names = append(names, item.Name)
	}
	require.Equal(t, []string{"vmcpi1agent", "vmcpi1pairelsewhere", "vmcpi1user"}, names)
}

// TestUserConfigurationNeverCarriesValues checks that the configuration a user enters, as the setup API shows it,
// carries no value, and leaves out what the catalog entry supplies itself.
func TestUserConfigurationNeverCarriesValues(t *testing.T) {
	component := pairComponents()[1]
	component.CatalogEntry.Manifest.Config[0].Value = "catalog-default"
	component.CatalogEntry.Manifest.Config = append(component.CatalogEntry.Manifest.Config, types.MCPConfig{Key: "STATIC", Static: true, Required: true})
	component.Configuration = append(component.Configuration, types.VMCPConfigurationPolicy{Key: "STATIC", Policy: types.VMCPConfigurationPolicyUserAllowed})

	fields := UserConfiguration(component)
	require.Len(t, fields, 1)
	require.Equal(t, "TOKEN", fields[0].Key)
	require.Empty(t, fields[0].Value)
}
