package vmcp

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	grantExplicit = "vp1explicit"
	grantWildcard = "vp1wildcard"
	grantOther    = "vp1otherwildcard"
	grantVMCP     = "vmcp1grants"
)

// memoryGrants stores profile OAuth grants in memory, keyed by owner and then MCPID, as the gateway database does:
// one grant per owner and component.
type memoryGrants struct {
	lock   sync.Mutex
	grants map[gatewaytypes.MCPOAuthTokenKey]gatewaytypes.MCPOAuthGrantMetadata
	// beforeDelete runs before DeleteMCPOAuthGrants deletes anything.
	beforeDelete func()
	// queries counts the calls that read grants.
	queries int
}

func newMemoryGrants() *memoryGrants {
	return &memoryGrants{grants: map[gatewaytypes.MCPOAuthTokenKey]gatewaytypes.MCPOAuthGrantMetadata{}}
}

func (m *memoryGrants) put(owner, mcpID, grantID, url string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	now := time.Now()
	m.grants[gatewaytypes.MCPOAuthTokenKey{UserID: owner, MCPID: mcpID}] = gatewaytypes.MCPOAuthGrantMetadata{
		UserID:       owner,
		MCPID:        mcpID,
		GrantID:      grantID,
		URL:          url,
		Scopes:       "read write",
		AuthorizedBy: "1",
		AuthorizedAt: &now,
	}
}

func (m *memoryGrants) invalidate(owner, mcpID string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	key := gatewaytypes.MCPOAuthTokenKey{UserID: owner, MCPID: mcpID}
	grant := m.grants[key]
	now := time.Now()
	grant.InvalidatedAt, grant.InvalidReason = &now, "refused"
	m.grants[key] = grant
}

func (m *memoryGrants) get(owner, mcpID string) (gatewaytypes.MCPOAuthGrantMetadata, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
	grant, ok := m.grants[gatewaytypes.MCPOAuthTokenKey{UserID: owner, MCPID: mcpID}]
	return grant, ok
}

func (m *memoryGrants) MCPOAuthGrantMetadata(_ context.Context, owners, mcpIDs []string) ([]gatewaytypes.MCPOAuthGrantMetadata, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.queries++
	var result []gatewaytypes.MCPOAuthGrantMetadata
	for key, grant := range m.grants {
		if slices.Contains(owners, key.UserID) && slices.Contains(mcpIDs, key.MCPID) {
			result = append(result, grant)
		}
	}
	return result, nil
}

func (m *memoryGrants) MCPOAuthGrantOwners(_ context.Context, owners []string) ([]string, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.queries++
	var result []string
	for key := range m.grants {
		if slices.Contains(owners, key.UserID) && !slices.Contains(result, key.UserID) {
			result = append(result, key.UserID)
		}
	}
	return result, nil
}

func (m *memoryGrants) DeleteMCPOAuthGrants(_ context.Context, owner, mcpID string, grantIDs []string) error {
	if m.beforeDelete != nil {
		m.beforeDelete()
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	key := gatewaytypes.MCPOAuthTokenKey{UserID: owner, MCPID: mcpID}
	if grant, ok := m.grants[key]; ok && slices.Contains(grantIDs, grant.GrantID) {
		delete(m.grants, key)
	}
	return nil
}

func (m *memoryGrants) DeleteMCPOAuthGrantsForOwners(_ context.Context, owners []string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	maps.DeleteFunc(m.grants, func(key gatewaytypes.MCPOAuthTokenKey, _ gatewaytypes.MCPOAuthGrantMetadata) bool {
		return slices.Contains(owners, key.UserID)
	})
	return nil
}

// oauthComponent returns a remote component whose upstream URL its definition fixes, which asks for OAuth when called.
func oauthComponent(id string) types.VMCPComponent {
	component := readinessComponent(id)
	component.CatalogEntry.Manifest.Runtime = types.RuntimeRemote
	component.CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{FixedURL: "https://" + id + ".example.com/mcp"}
	return component
}

// grantTestVMCP returns a shared vMCP whose components are a static OAuth remote, a remote that asks for OAuth when
// called, and a server that does not use OAuth, with a profile for agent 7 and one for every agent.
func grantTestVMCP(profiles ...types.VMCPProfile) v1.VMCP {
	if len(profiles) == 0 {
		profiles = []types.VMCPProfile{explicitAgentProfile(grantExplicit, "7"), everyAgentProfile(grantWildcard, types.SubjectTypeAgent)}
	}
	return v1.VMCP{
		Name:      grantVMCP,
		Namespace: "default",
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			DisplayName: "Grants",
			Components:  []types.VMCPComponent{staticOAuthComponent("static"), oauthComponent("dynamic"), readinessComponent("local")},
			Profiles:    profiles,
		}},
	}
}

func grantOwner(profileID string) string {
	return principal.ProfileGrant(grantVMCP, profileID)
}

func grantMCPID(componentID string) string {
	return ProfileOAuthGrantMCPID(grantVMCP, componentID)
}

func TestSelectAgentGrants(t *testing.T) {
	staticURL, dynamicURL := "https://static.example.com/mcp", "https://dynamic.example.com/mcp"
	for _, tt := range []struct {
		name     string
		profiles []types.VMCPProfile
		agent    uint
		// stored are the grants stored, by owner profile and then component.
		stored map[string]map[string]string
		// invalid are the stored grants marked invalid, by owner profile and then component.
		invalid map[string][]string
		// want is the profile whose grant the agent uses for each component, by component, and wantFallback those it
		// falls back for.
		want         map[string]string
		wantFallback []string
		wantReasons  []ReadinessReason
	}{
		{
			name:   "explicit grant",
			agent:  7,
			stored: map[string]map[string]string{grantExplicit: {"static": staticURL, "dynamic": dynamicURL}, grantWildcard: {"static": staticURL, "dynamic": dynamicURL}},
			want:   map[string]string{"static": grantExplicit, "dynamic": grantExplicit},
		},
		{
			name:         "explicit grant missing falls back",
			agent:        7,
			stored:       map[string]map[string]string{grantExplicit: {"dynamic": dynamicURL}, grantWildcard: {"static": staticURL}},
			want:         map[string]string{"static": grantWildcard, "dynamic": grantExplicit},
			wantFallback: []string{"static"},
		},
		{
			name:         "agent without an explicit profile uses the wildcard",
			agent:        9,
			stored:       map[string]map[string]string{grantExplicit: {"static": staticURL}, grantWildcard: {"static": staticURL}},
			want:         map[string]string{"static": grantWildcard},
			wantFallback: []string{"static"},
		},
		{
			name:        "broken explicit grant never falls back",
			agent:       7,
			stored:      map[string]map[string]string{grantExplicit: {"static": staticURL}, grantWildcard: {"static": staticURL}},
			invalid:     map[string][]string{grantExplicit: {"static"}},
			want:        map[string]string{"static": grantExplicit},
			wantReasons: []ReadinessReason{{ComponentID: "static", Code: ReadinessOAuthInvalid, ProfileID: grantExplicit}},
		},
		{
			name:        "explicit grant for another target never falls back",
			agent:       7,
			stored:      map[string]map[string]string{grantExplicit: {"static": "https://moved.example.com/mcp"}, grantWildcard: {"static": staticURL}},
			want:        map[string]string{"static": grantExplicit},
			wantReasons: []ReadinessReason{{ComponentID: "static", Code: ReadinessOAuthInvalid, ProfileID: grantExplicit}},
		},
		{
			name:         "broken wildcard grant",
			agent:        9,
			stored:       map[string]map[string]string{grantWildcard: {"dynamic": dynamicURL}},
			invalid:      map[string][]string{grantWildcard: {"dynamic"}},
			want:         map[string]string{"dynamic": grantWildcard},
			wantFallback: []string{"dynamic"},
			wantReasons: []ReadinessReason{
				{ComponentID: "static", Code: ReadinessOAuthRequired, ProfileID: grantWildcard},
				{ComponentID: "dynamic", Code: ReadinessOAuthInvalid, ProfileID: grantWildcard},
			},
		},
		{
			// Only a component known to need OAuth is reported without a grant; the other is caught when it is called.
			name:        "no grant",
			agent:       7,
			wantReasons: []ReadinessReason{{ComponentID: "static", Code: ReadinessOAuthRequired, ProfileID: grantExplicit}},
		},
		{
			name:     "explicit grant on an ambiguous vMCP",
			profiles: []types.VMCPProfile{explicitAgentProfile(grantExplicit, "7"), legacyWildcardProfile(grantWildcard), legacyWildcardProfile(grantOther)},
			agent:    7,
			stored:   map[string]map[string]string{grantExplicit: {"static": staticURL}, grantWildcard: {"dynamic": dynamicURL}},
			want:     map[string]string{"static": grantExplicit},
		},
		{
			name:        "explicit grant missing on an ambiguous vMCP",
			profiles:    []types.VMCPProfile{explicitAgentProfile(grantExplicit, "7"), legacyWildcardProfile(grantWildcard), legacyWildcardProfile(grantOther)},
			agent:       7,
			stored:      map[string]map[string]string{grantWildcard: {"static": staticURL}},
			wantReasons: []ReadinessReason{AmbiguousFallbackReason("static")},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vmcp := grantTestVMCP(tt.profiles...)
			grants := newMemoryGrants()
			for profileID, components := range tt.stored {
				for componentID, url := range components {
					grants.put(grantOwner(profileID), grantMCPID(componentID), "grant-"+profileID+"-"+componentID, url)
				}
			}
			for profileID, components := range tt.invalid {
				for _, componentID := range components {
					grants.invalidate(grantOwner(profileID), grantMCPID(componentID))
				}
			}
			store := NewProfileOAuthGrantStore(grants)
			agent := agentInfo(tt.agent, nil)
			selection := SelectAgentProfiles(agent, vmcp.Spec.Manifest)

			selected, err := store.SelectAgentGrants(t.Context(), vmcp, selection, vmcp.Spec.Manifest.Components)
			require.NoError(t, err)
			require.Equal(t, 1, grants.queries, "selection reads every component's grants with one query")
			got := map[string]string{}
			var fallback []string
			for _, grant := range selected {
				if grant.ProfileID != "" {
					got[grant.ComponentID] = grant.ProfileID
					require.Equal(t, grantOwner(grant.ProfileID), grant.Owner)
					require.Equal(t, grantMCPID(grant.ComponentID), grant.MCPID)
				}
				if grant.Fallback {
					fallback = append(fallback, grant.ComponentID)
				}
			}
			if tt.want == nil {
				tt.want = map[string]string{}
			}
			require.Equal(t, tt.want, got)
			require.ElementsMatch(t, tt.wantFallback, fallback)
			var wantFallbackProfiles []string
			for _, componentID := range tt.wantFallback {
				if !slices.Contains(wantFallbackProfiles, tt.want[componentID]) {
					wantFallbackProfiles = append(wantFallbackProfiles, tt.want[componentID])
				}
			}
			require.Equal(t, wantFallbackProfiles, FallbackGrantProfileIDs(selected))

			readiness, err := AgentReadinessChecks{OAuth: ProfileOAuthGrantCheck{Grants: store}}.AgentReadiness(t.Context(), vmcp, v1.VMCPInstance{}, agent)
			require.NoError(t, err)
			require.Equal(t, tt.wantReasons, readiness.Reasons)
			if slices.ContainsFunc(tt.wantReasons, func(reason ReadinessReason) bool { return reason.Code == ReadinessOAuthInvalid }) {
				require.Equal(t, AgentOAuthRequiredError, readiness.ErrorCode())
			}
		})
	}
}

// newGrantTestStorage returns storage holding vmcp, with its profiles recorded in status, which keeps status apart
// from the spec and checks resource versions on both, as the API server does.
func newGrantTestStorage(t *testing.T, vmcp v1.VMCP) (kclient.Client, kclient.ObjectKey) {
	t.Helper()
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(&v1.VMCP{}).WithObjects(&vmcp).Build()
	key := kclient.ObjectKeyFromObject(&vmcp)
	current := &v1.VMCP{}
	require.NoError(t, client.Get(t.Context(), key, current))
	current.Status.ProfileIDs = ProfileIDs(current.Spec.Manifest)
	require.NoError(t, client.Status().Update(t.Context(), current))
	return client, key
}

func storedVMCP(t *testing.T, client kclient.Client, key kclient.ObjectKey) *v1.VMCP {
	t.Helper()
	current := &v1.VMCP{}
	require.NoError(t, client.Get(t.Context(), key, current))
	return current
}

func noGrantCheck(*v1.VMCP, types.VMCPProfile, types.VMCPComponent) error {
	return nil
}

// TestProfileOAuthGrantAuthorize checks that a grant is recorded in the vMCP's status, with its profile, before it is
// stored, and then published as the only grant of its component; and which profiles and components can hold one.
func TestProfileOAuthGrantAuthorize(t *testing.T) {
	vmcp := grantTestVMCP(explicitAgentProfile(grantExplicit, "7"), everyAgentProfile(grantWildcard, types.SubjectTypeAgent), everyAgentProfile("vp1users", types.SubjectTypeUser))
	client, key := newGrantTestStorage(t, vmcp)
	// The profile was just added, and the controller has not recorded it yet.
	current := storedVMCP(t, client, key)
	current.Status.ProfileIDs = []string{grantWildcard}
	require.NoError(t, client.Status().Update(t.Context(), current))

	grants := newMemoryGrants()
	store := NewProfileOAuthGrantStore(grants)
	var written []string
	write := func(_ context.Context, owner, mcpID, grantID string) error {
		stored := storedVMCP(t, client, key)
		require.Contains(t, stored.Status.ProfileIDs, grantExplicit, "the grant was stored before its profile was recorded")
		require.Contains(t, recordedGrantIDs(stored, grantExplicit, "static"), grantID, "the grant was stored before it was recorded")
		grants.put(owner, mcpID, grantID, "https://static.example.com/mcp")
		written = append(written, grantID)
		return nil
	}
	updated, err := store.Authorize(t.Context(), client, key, grantExplicit, "static", noGrantCheck, write)
	require.NoError(t, err)
	require.Len(t, written, 1)
	require.Equal(t, []v1.VMCPProfileOAuthGrant{{ProfileID: grantExplicit, ComponentID: "static", GrantIDs: written}}, updated.Status.ProfileOAuthGrants)
	stored, ok := grants.get(grantOwner(grantExplicit), grantMCPID("static"))
	require.True(t, ok)
	require.Equal(t, written[0], stored.GrantID)

	// Authorizing again replaces the grant, and only the new one stays recorded.
	updated, err = store.Authorize(t.Context(), client, key, grantExplicit, "static", noGrantCheck, write)
	require.NoError(t, err)
	require.Len(t, written, 2)
	require.Equal(t, []v1.VMCPProfileOAuthGrant{{ProfileID: grantExplicit, ComponentID: "static", GrantIDs: written[1:]}}, updated.Status.ProfileOAuthGrants)

	// The check is asked with what is stored, and refuses the grant.
	_, err = store.Authorize(t.Context(), client, key, grantExplicit, "dynamic", func(_ *v1.VMCP, profile types.VMCPProfile, component types.VMCPComponent) error {
		require.Equal(t, grantExplicit, profile.ID)
		require.Equal(t, "dynamic", component.ID)
		return types.NewErrHTTP(http.StatusConflict, "changed")
	}, write)
	requireHTTPError(t, err, http.StatusConflict, "changed")

	for _, tt := range []struct {
		name      string
		profile   string
		component string
		code      int
		contains  string
	}{
		{name: "profile for users", profile: "vp1users", component: "static", code: http.StatusBadRequest, contains: "does not apply to agents"},
		{name: "unknown profile", profile: "vp1missing", component: "static", code: http.StatusNotFound, contains: "no profile"},
		{name: "unknown component", profile: grantExplicit, component: "missing", code: http.StatusNotFound, contains: "no component"},
		{name: "component without OAuth", profile: grantExplicit, component: "local", code: http.StatusBadRequest, contains: "not a remote server"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := store.Authorize(t.Context(), client, key, tt.profile, tt.component, noGrantCheck, write)
			requireHTTPError(t, err, tt.code, tt.contains)
		})
	}
	require.Len(t, written, 2, "a refused grant was stored")

	// No grant is written while agent selection is ambiguous.
	ambiguous := storedVMCP(t, client, key)
	ambiguous.Spec.Manifest.Profiles = append(ambiguous.Spec.Manifest.Profiles, legacyWildcardProfile(grantOther), legacyWildcardProfile("vp1third"))
	require.NoError(t, client.Update(t.Context(), ambiguous))
	_, err = store.Authorize(t.Context(), client, key, grantExplicit, "static", noGrantCheck, write)
	requireHTTPError(t, err, http.StatusConflict, "more than one profile")

	// The stored status names grants, never their tokens.
	statusJSON, err := json.Marshal(storedVMCP(t, client, key).Status)
	require.NoError(t, err)
	require.NotContains(t, string(statusJSON), "token")
}

// TestProfileOAuthGrantAuthorizeDeletesUnpublishedGrant removes the profile while its grant is being stored: the grant
// is refused, and leaves nothing behind, even though the controller deleted the profile's data before it was stored.
func TestProfileOAuthGrantAuthorizeDeletesUnpublishedGrant(t *testing.T) {
	client, key := newGrantTestStorage(t, grantTestVMCP())
	grants := newMemoryGrants()
	store := NewProfileOAuthGrantStore(grants)
	var attempts int
	write := func(_ context.Context, owner, mcpID, grantID string) error {
		attempts++
		current := storedVMCP(t, client, key)
		current.Spec.Manifest.Profiles = current.Spec.Manifest.Profiles[1:]
		require.NoError(t, client.Update(t.Context(), current))
		// The controller deletes the removed profile's data before this write stores the grant.
		require.NoError(t, store.DeleteProfileData(t.Context(), key.Name, []string{grantExplicit}))
		grants.put(owner, mcpID, grantID, "https://static.example.com/mcp")
		return nil
	}

	_, err := store.Authorize(t.Context(), client, key, grantExplicit, "static", noGrantCheck, write)
	requireHTTPError(t, err, http.StatusNotFound, "no profile")
	require.Equal(t, 1, attempts)
	_, ok := grants.get(grantOwner(grantExplicit), grantMCPID("static"))
	require.False(t, ok, "the unpublished grant was left behind")
}

// TestProfileOAuthGrantReset checks that a reset deletes the profile's grant and its record, that it is allowed where
// writing is not, and that it deletes only the grants it listed: one authorized after it listed them is kept.
func TestProfileOAuthGrantReset(t *testing.T) {
	client, key := newGrantTestStorage(t, grantTestVMCP())
	grants := newMemoryGrants()
	store := NewProfileOAuthGrantStore(grants)
	write := func(_ context.Context, owner, mcpID, grantID string) error {
		grants.put(owner, mcpID, grantID, "https://static.example.com/mcp")
		return nil
	}
	for _, profileID := range []string{grantExplicit, grantWildcard} {
		_, err := store.Authorize(t.Context(), client, key, profileID, "static", noGrantCheck, write)
		require.NoError(t, err)
	}

	updated, cleared, err := store.Reset(t.Context(), client, key, grantExplicit, "static")
	require.NoError(t, err)
	require.True(t, cleared)
	require.Empty(t, recordedGrantIDs(updated, grantExplicit, "static"))
	_, ok := grants.get(grantOwner(grantExplicit), grantMCPID("static"))
	require.False(t, ok)
	// The wildcard's grant for the same component is another profile's.
	_, ok = grants.get(grantOwner(grantWildcard), grantMCPID("static"))
	require.True(t, ok)

	_, cleared, err = store.Reset(t.Context(), client, key, grantExplicit, "static")
	require.NoError(t, err)
	require.False(t, cleared)

	// An administrator authorizes the grant again while a reset is deleting it: the new grant has an ID the reset did
	// not list, and is kept.
	grants.beforeDelete = func() {
		grants.beforeDelete = nil
		grants.put(grantOwner(grantWildcard), grantMCPID("static"), "grant-after-listing", "https://static.example.com/mcp")
	}
	_, cleared, err = store.Reset(t.Context(), client, key, grantWildcard, "static")
	require.NoError(t, err)
	require.True(t, cleared)
	kept, ok := grants.get(grantOwner(grantWildcard), grantMCPID("static"))
	require.True(t, ok)
	require.Equal(t, "grant-after-listing", kept.GrantID)

	// A reset is allowed on a profile that no longer applies to agents, and for a component the vMCP no longer has.
	current := storedVMCP(t, client, key)
	current.Spec.Manifest.Profiles[0].Subjects = []types.Subject{{Type: types.SubjectTypeUser, ID: "3"}}
	current.Spec.Manifest.Components = current.Spec.Manifest.Components[1:]
	require.NoError(t, client.Update(t.Context(), current))
	grants.put(grantOwner(grantExplicit), grantMCPID("static"), "grant-left", "https://static.example.com/mcp")
	_, cleared, err = store.Reset(t.Context(), client, key, grantExplicit, "static")
	require.NoError(t, err)
	require.True(t, cleared)
	_, ok = grants.get(grantOwner(grantExplicit), grantMCPID("static"))
	require.False(t, ok)

	_, _, err = store.Reset(t.Context(), client, key, "vp1missing", "static")
	requireHTTPError(t, err, http.StatusNotFound, "no profile")
}

// TestDeleteRemovedComponentGrants checks that the grants of a component removed from the vMCP are deleted by their
// recorded IDs, and their records with them; that other components' grants are kept; and that a grant written since,
// for a component of the same ID added back, has an ID the records do not hold, and is kept too.
func TestDeleteRemovedComponentGrants(t *testing.T) {
	vmcp := grantTestVMCP()
	vmcp.Status.ProfileOAuthGrants = []v1.VMCPProfileOAuthGrant{
		{ProfileID: grantExplicit, ComponentID: "static", GrantIDs: []string{"grant-static"}},
		{ProfileID: grantWildcard, ComponentID: "dynamic", GrantIDs: []string{"grant-dynamic", "grant-pending"}},
		{ProfileID: grantWildcard, ComponentID: "removed", GrantIDs: []string{"grant-removed"}},
		{ProfileID: grantExplicit, ComponentID: "readded", GrantIDs: []string{"grant-before-removal"}},
	}
	grants := newMemoryGrants()
	grants.put(grantOwner(grantExplicit), grantMCPID("static"), "grant-static", "https://static.example.com/mcp")
	grants.put(grantOwner(grantWildcard), grantMCPID("dynamic"), "grant-pending", "https://dynamic.example.com/mcp")
	grants.put(grantOwner(grantWildcard), grantMCPID("removed"), "grant-removed", "https://removed.example.com/mcp")
	grants.put(grantOwner(grantExplicit), grantMCPID("readded"), "grant-after-readding", "https://readded.example.com/mcp")
	store := NewProfileOAuthGrantStore(grants)

	changed, err := store.DeleteRemovedComponentGrants(t.Context(), &vmcp)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []v1.VMCPProfileOAuthGrant{
		{ProfileID: grantExplicit, ComponentID: "static", GrantIDs: []string{"grant-static"}},
		{ProfileID: grantWildcard, ComponentID: "dynamic", GrantIDs: []string{"grant-dynamic", "grant-pending"}},
	}, vmcp.Status.ProfileOAuthGrants)
	for _, tt := range []struct {
		profileID   string
		componentID string
		kept        bool
	}{
		{profileID: grantExplicit, componentID: "static", kept: true},
		{profileID: grantWildcard, componentID: "dynamic", kept: true},
		{profileID: grantWildcard, componentID: "removed"},
		{profileID: grantExplicit, componentID: "readded", kept: true},
	} {
		_, ok := grants.get(grantOwner(tt.profileID), grantMCPID(tt.componentID))
		require.Equal(t, tt.kept, ok, "grant of %s for %s", tt.profileID, tt.componentID)
	}

	changed, err = store.DeleteRemovedComponentGrants(t.Context(), &vmcp)
	require.NoError(t, err)
	require.False(t, changed)
}

// TestProfileOAuthGrantStoreIsAProfileDataStore checks the profile data hook with real grants: which profiles hold
// grants, in one query, and deleting them, which can be repeated and leaves other vMCPs' profiles alone.
func TestProfileOAuthGrantStoreIsAProfileDataStore(t *testing.T) {
	grants := newMemoryGrants()
	grants.put(grantOwner(grantExplicit), grantMCPID("static"), "a", "https://static.example.com/mcp")
	grants.put(grantOwner(grantExplicit), grantMCPID("dynamic"), "b", "https://dynamic.example.com/mcp")
	grants.put(grantOwner(grantWildcard), grantMCPID("static"), "c", "https://static.example.com/mcp")
	// Another vMCP's profile of the same ID is another profile, and a user's token is never a grant.
	grants.put(principal.ProfileGrant("vmcp1other", grantExplicit), ProfileOAuthGrantMCPID("vmcp1other", "static"), "d", "https://static.example.com/mcp")
	grants.put("7", "msi1connection", "e", "https://static.example.com/mcp")
	store := NewProfileOAuthGrantStore(grants)

	queries := grants.queries
	withData, err := store.ProfilesWithData(t.Context(), grantVMCP, []string{"vp1missing", grantWildcard, grantExplicit})
	require.NoError(t, err)
	require.Equal(t, []string{grantWildcard, grantExplicit}, withData)
	require.Equal(t, queries+1, grants.queries)

	for range 2 {
		require.NoError(t, store.DeleteProfileData(t.Context(), grantVMCP, []string{grantExplicit}))
	}
	withData, err = store.ProfilesWithData(t.Context(), grantVMCP, []string{grantWildcard, grantExplicit})
	require.NoError(t, err)
	require.Equal(t, []string{grantWildcard}, withData)
	_, ok := grants.get(principal.ProfileGrant("vmcp1other", grantExplicit), ProfileOAuthGrantMCPID("vmcp1other", "static"))
	require.True(t, ok)
	_, ok = grants.get("7", "msi1connection")
	require.True(t, ok)
}

// TestProfileOAuthStatus checks the status of a profile's grants: every component that can hold one, with who
// authorized it, when, for which target and scopes, and no fallback to another profile.
func TestProfileOAuthStatus(t *testing.T) {
	vmcp := grantTestVMCP()
	grants := newMemoryGrants()
	grants.put(grantOwner(grantExplicit), grantMCPID("static"), "a", "https://static.example.com/mcp")
	grants.put(grantOwner(grantExplicit), grantMCPID("dynamic"), "b", "https://dynamic.example.com/mcp")
	grants.invalidate(grantOwner(grantExplicit), grantMCPID("dynamic"))
	grants.put(grantOwner(grantWildcard), grantMCPID("static"), "c", "https://static.example.com/mcp")
	store := NewProfileOAuthGrantStore(grants)

	explicit, err := store.ProfileOAuthStatus(t.Context(), vmcp, grantExplicit)
	require.NoError(t, err)
	require.Len(t, explicit, 2, "a component that does not use OAuth has no grant status")
	static := explicit["static"]
	require.NotNil(t, static.AuthorizedAt)
	static.AuthorizedAt = nil
	require.Equal(t, types.VMCPProfileOAuthStatus{
		Required:     true,
		State:        types.VMCPProfileOAuthAuthorized,
		AuthorizedBy: "1",
		Target:       "https://static.example.com/mcp",
		Scopes:       []string{"read", "write"},
	}, static)
	require.Equal(t, types.VMCPProfileOAuthInvalid, explicit["dynamic"].State)
	require.Equal(t, "refused", explicit["dynamic"].InvalidReason)
	require.False(t, explicit["dynamic"].Required)

	// The profile alone: the wildcard's grant is not the users profile's.
	users, err := store.ProfileOAuthStatus(t.Context(), vmcp, "vp1users")
	require.NoError(t, err)
	require.Equal(t, types.VMCPProfileOAuthMissing, users["static"].State)
	require.Equal(t, types.VMCPProfileOAuthMissing, users["dynamic"].State)

	status := ProfileStatus(vmcp, vmcp.Spec.Manifest.Profiles[0])
	AddOAuthStatus(&status, explicit)
	require.Nil(t, status.Components[2].OAuth)
	require.Equal(t, types.VMCPProfileOAuthAuthorized, status.Components[0].OAuth.State)
}

// TestProfileGrantOwnerIsNeverACaller checks that a caller whose UID is a profile's grant owner, however it presents
// itself, matches no profile, not even the one that names everyone, owns no instance, and is not an agent, so it can
// never reach a vMCP, and so never the grants, as anyone.
func TestProfileGrantOwnerIsNeverACaller(t *testing.T) {
	vmcp := grantTestVMCP(legacyWildcardProfile(grantWildcard), everyAgentProfile(grantOther, types.SubjectTypeAgent))
	owner := grantOwner(grantWildcard)
	for _, kind := range []principal.Kind{principal.KindUser, principal.KindAgent, ""} {
		extra := map[string][]string{}
		if kind != "" {
			principal.RecordKind(extra, kind)
		}
		caller := agentInfo(1, nil)
		caller.UID, caller.Name, caller.Extra = owner, owner, extra
		require.Empty(t, MatchingProfiles(caller, vmcp.Spec.Manifest.Profiles), "kind %q", kind)
		require.False(t, principal.IsAgent(caller), "kind %q", kind)
		_, err := principal.CallerOwner(caller)
		require.Error(t, err, "kind %q", kind)
		_, err = AgentReadiness(t.Context(), vmcp, v1.VMCPInstance{}, caller)
		require.Error(t, err, "kind %q", kind)
	}
}

// TestProfileGrantSelection checks the agent a profile's grants are bound for: one whose explicit profile is the
// profile, falling back to the wildcard profile, or one that has only the wildcard profile when the profile is it.
func TestProfileGrantSelection(t *testing.T) {
	explicit, wildcard := explicitAgentProfile(grantExplicit, "7"), everyAgentProfile(grantWildcard, types.SubjectTypeAgent)
	manifest := types.VMCPManifest{Profiles: []types.VMCPProfile{explicit, wildcard}}

	selection := ProfileGrantSelection(manifest, explicit)
	require.Equal(t, grantExplicit, selection.Explicit.ID)
	require.Equal(t, grantWildcard, selection.Wildcard.ID)
	require.False(t, selection.Ambiguous)

	selection = ProfileGrantSelection(manifest, wildcard)
	require.Nil(t, selection.Explicit)
	require.Equal(t, grantWildcard, selection.Wildcard.ID)

	// Without a wildcard profile, the profile has no fallback; with several, its fallback is ambiguous.
	selection = ProfileGrantSelection(types.VMCPManifest{Profiles: []types.VMCPProfile{explicit}}, explicit)
	require.Equal(t, grantExplicit, selection.Explicit.ID)
	require.Nil(t, selection.Wildcard)
	require.False(t, selection.Ambiguous)
	selection = ProfileGrantSelection(types.VMCPManifest{Profiles: []types.VMCPProfile{explicit, legacyWildcardProfile(grantWildcard), legacyWildcardProfile(grantOther)}}, explicit)
	require.Nil(t, selection.Wildcard)
	require.True(t, selection.Ambiguous)
}
