package vmcp

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	// grantAgent7 names agent 7, grantAgent9 agent 9, and grantUsers every user. The first two grant every component.
	grantAgent7 = "vp1agent7"
	grantAgent9 = "vp1agent9"
	grantUsers  = "vp1users"
	grantVMCP   = "vmcp1grants"
)

// memoryGrants stores preconfigured OAuth grants in memory, keyed by owner and then MCPID, as the gateway database
// does: one grant per owner and component.
type memoryGrants struct {
	lock   sync.Mutex
	grants map[gatewaytypes.MCPOAuthTokenKey]gatewaytypes.MCPOAuthGrantMetadata
	// beforeDelete runs before DeleteMCPOAuthGrants deletes anything.
	beforeDelete func()
	// queries counts the calls that read grants.
	queries int
}

// fakeTargets resolves every component's URL to url, refuses it with refused, or fails with err.
type fakeTargets struct {
	url     string
	missing []string
	refused error
	err     error
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

func (m *memoryGrants) DeleteMCPOAuthGrantsForOwner(_ context.Context, owner string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	maps.DeleteFunc(m.grants, func(key gatewaytypes.MCPOAuthTokenKey, _ gatewaytypes.MCPOAuthGrantMetadata) bool {
		return key.UserID == owner
	})
	return nil
}

func (f fakeTargets) ComponentTarget(context.Context, v1.VMCP, types.VMCPComponent) (ComponentTarget, error) {
	switch {
	case f.refused != nil:
		return ComponentTarget{}, f.refused
	case f.err != nil:
		return ComponentTarget{}, f.err
	}
	return ComponentTarget{URL: f.url, Missing: f.missing}, nil
}

// oauthComponent returns a remote component whose upstream URL its definition fixes, which asks for OAuth when called.
func oauthComponent(id string) types.VMCPComponent {
	component := readinessComponent(id)
	component.CatalogEntry.Manifest.Runtime = types.RuntimeRemote
	component.CatalogEntry.Manifest.RemoteConfig = &types.RemoteCatalogConfig{FixedURL: "https://" + id + ".example.com/mcp"}
	return component
}

// grantTestVMCP returns a shared vMCP whose components are a static OAuth remote, a remote that asks for OAuth when
// called, and a server that does not use OAuth, with profiles, by default one for agent 7, one for agent 9, and one for
// every user, each granting every component.
func grantTestVMCP(profiles ...types.VMCPProfile) v1.VMCP {
	if len(profiles) == 0 {
		profiles = []types.VMCPProfile{agentNamingProfile(grantAgent7, "7"), agentNamingProfile(grantAgent9, "9"), everyUserProfile(grantUsers)}
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

func requireHTTPError(t *testing.T, err error, code int, contains string) {
	t.Helper()
	httpErr, ok := err.(*types.ErrHTTP)
	require.True(t, ok, "error %v (%T) is not an API error", err, err)
	require.Equal(t, code, httpErr.Code, httpErr.Message)
	require.Contains(t, httpErr.Message, contains)
}

// grantOwner is the owner of the test vMCP's grants.
func grantOwner() string {
	return principal.PreconfiguredOAuthOwner(grantVMCP)
}

func grantMCPID(componentID string) string {
	return PreconfiguredOAuthMCPID(grantVMCP, componentID)
}

// TestSelectGrants checks the selection of the vMCP's grant for each component, with one query, and the readiness of an
// agent granted every component that follows from it: a grant that works, one that is invalid or for another target,
// and none, for a component known to need OAuth and one that is not.
func TestSelectGrants(t *testing.T) {
	staticURL, dynamicURL := "https://static.example.com/mcp", "https://dynamic.example.com/mcp"
	for _, tt := range []struct {
		name string
		// stored are the vMCP's grants, by component, with the URL each was authorized for.
		stored map[string]string
		// invalid are the components whose stored grant is marked invalid.
		invalid []string
		// wantHeld are the components the vMCP holds a grant for.
		wantHeld    []string
		wantReasons []ReadinessReason
	}{
		{
			name: "a grant for every component that can hold one",
			stored: map[string]string{
				"static":  staticURL,
				"dynamic": dynamicURL,
			},
			wantHeld: []string{"static", "dynamic"},
		},
		{
			// Only a component known to need OAuth is reported without a grant; the other is caught when it is called.
			name: "no grant",
			wantReasons: []ReadinessReason{{
				ComponentID: "static",
				Code:        types.AgentReadinessOAuthRequired,
			}},
		},
		{
			name: "a broken grant",
			stored: map[string]string{
				"static": staticURL,
			},
			invalid:  []string{"static"},
			wantHeld: []string{"static"},
			wantReasons: []ReadinessReason{{
				ComponentID: "static",
				Code:        types.AgentReadinessOAuthInvalid,
			}},
		},
		{
			name: "a grant for another target",
			stored: map[string]string{
				"static": "https://moved.example.com/mcp",
			},
			wantHeld: []string{"static"},
			wantReasons: []ReadinessReason{{
				ComponentID: "static",
				Code:        types.AgentReadinessOAuthInvalid,
			}},
		},
		{
			// A component that asks for OAuth only once it is called, which the vMCP holds a grant for, needs it: the
			// grant could only be authorized for a server that asked for OAuth.
			name: "a late challenge the vMCP holds a grant for",
			stored: map[string]string{
				"dynamic": dynamicURL,
			},
			wantHeld: []string{"dynamic"},
			wantReasons: []ReadinessReason{{
				ComponentID: "static",
				Code:        types.AgentReadinessOAuthRequired,
			}},
		},
		{
			name: "a late challenge the vMCP holds a broken grant for",
			stored: map[string]string{
				"dynamic": dynamicURL,
			},
			invalid:  []string{"dynamic"},
			wantHeld: []string{"dynamic"},
			wantReasons: []ReadinessReason{
				{
					ComponentID: "static",
					Code:        types.AgentReadinessOAuthRequired,
				},
				{
					ComponentID: "dynamic",
					Code:        types.AgentReadinessOAuthInvalid,
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vmcp := grantTestVMCP()
			grants := newMemoryGrants()
			for componentID, url := range tt.stored {
				grants.put(grantOwner(), grantMCPID(componentID), "grant-"+componentID, url)
			}
			for _, componentID := range tt.invalid {
				grants.invalidate(grantOwner(), grantMCPID(componentID))
			}
			store := NewPreconfiguredOAuthStore(grants)

			selected, err := store.SelectGrants(t.Context(), vmcp, vmcp.Spec.Manifest.Components)
			require.NoError(t, err)
			require.Equal(t, 1, grants.queries, "selection reads every component's grant with one query")
			held := []string{}
			for _, grant := range selected {
				if grant.Held() {
					held = append(held, grant.ComponentID)
					require.Equal(t, grantOwner(), grant.Owner)
					require.Equal(t, grantMCPID(grant.ComponentID), grant.MCPID)
				}
			}
			if tt.wantHeld == nil {
				tt.wantHeld = []string{}
			}
			require.Equal(t, tt.wantHeld, held)
			// A component that does not use OAuth never holds a grant, and never needs one.
			require.False(t, selected[2].Held())
			require.False(t, selected[2].Required)

			for _, agent := range []uint{7, 9} {
				readiness, err := readinessWith(t, store, vmcp, agentInfo(agent, nil))
				require.NoError(t, err)
				require.Equal(t, tt.wantReasons, readiness.Reasons, "agent %d", agent)
				if len(tt.wantReasons) > 0 {
					require.Equal(t, AgentOAuthRequiredError, readiness.ErrorCode())
				}
			}
		})
	}
}

// TestSelectGrantsReadsOnlyTheVMCPsGrants checks that the only grant selected for a vMCP's component is the one stored
// under the vMCP's own grant owner: never another vMCP's, a user's or an agent's own token for the same MCPID, or a grant
// stored under a profile, as grants were before they moved to the vMCP.
func TestSelectGrantsReadsOnlyTheVMCPsGrants(t *testing.T) {
	vmcp := grantTestVMCP()
	grants := newMemoryGrants()
	for _, owner := range []string{
		principal.PreconfiguredOAuthOwner("vmcp1other"),
		"7",
		"agent:7",
		"vmcp-profile:" + grantVMCP + "/" + grantAgent7,
	} {
		grants.put(owner, grantMCPID("static"), "grant-"+owner, "https://static.example.com/mcp")
	}
	store := NewPreconfiguredOAuthStore(grants)

	selected, err := store.SelectGrants(t.Context(), vmcp, vmcp.Spec.Manifest.Components)
	require.NoError(t, err)
	for _, grant := range selected {
		require.False(t, grant.Held(), "a grant of another owner was selected for %s", grant.ComponentID)
	}
	readiness, err := readinessWith(t, store, vmcp, agentInfo(7, nil))
	require.NoError(t, err)
	require.Equal(t, []ReadinessReason{{ComponentID: "static", Code: types.AgentReadinessOAuthRequired}}, readiness.Reasons)
}

// newGrantTestStorage returns storage holding vmcp, which keeps status apart from the spec and checks resource versions
// on both, as the API server does.
func newGrantTestStorage(t *testing.T, vmcp v1.VMCP) (kclient.Client, kclient.ObjectKey) {
	t.Helper()
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(&v1.VMCP{}).WithObjects(&vmcp).Build()
	return client, kclient.ObjectKeyFromObject(&vmcp)
}

func storedVMCP(t *testing.T, client kclient.Client, key kclient.ObjectKey) *v1.VMCP {
	t.Helper()
	current := &v1.VMCP{}
	require.NoError(t, client.Get(t.Context(), key, current))
	return current
}

func noGrantCheck(*v1.VMCP, types.VMCPComponent) error {
	return nil
}

// TestPreconfiguredOAuthAuthorize checks that a grant is recorded in the vMCP's status before it is stored, and then
// published as the only grant of its component, and that a reauthorization replaces it.
func TestPreconfiguredOAuthAuthorize(t *testing.T) {
	client, key := newGrantTestStorage(t, grantTestVMCP())
	grants := newMemoryGrants()
	store := NewPreconfiguredOAuthStore(grants)
	var written []string
	write := func(_ context.Context, owner, mcpID, grantID string) error {
		require.Contains(t, recordedGrantIDs(storedVMCP(t, client, key), "static"), grantID, "the grant was stored before it was recorded")
		grants.put(owner, mcpID, grantID, "https://static.example.com/mcp")
		written = append(written, grantID)
		return nil
	}
	updated, err := store.Authorize(t.Context(), client, key, "static", noGrantCheck, write)
	require.NoError(t, err)
	require.Len(t, written, 1)
	require.Equal(t, []v1.VMCPPreconfiguredOAuthGrant{{ComponentID: "static", GrantIDs: written}}, updated.Status.PreconfiguredOAuth)
	stored, ok := grants.get(grantOwner(), grantMCPID("static"))
	require.True(t, ok)
	require.Equal(t, written[0], stored.GrantID)

	// Authorizing again replaces the grant, and only the new one stays recorded.
	updated, err = store.Authorize(t.Context(), client, key, "static", noGrantCheck, write)
	require.NoError(t, err)
	require.Len(t, written, 2)
	require.Equal(t, []v1.VMCPPreconfiguredOAuthGrant{{ComponentID: "static", GrantIDs: written[1:]}}, updated.Status.PreconfiguredOAuth)

	// The check is asked with what is stored, and refuses the grant.
	_, err = store.Authorize(t.Context(), client, key, "dynamic", func(_ *v1.VMCP, component types.VMCPComponent) error {
		require.Equal(t, "dynamic", component.ID)
		return types.NewErrHTTP(http.StatusConflict, "changed")
	}, write)
	requireHTTPError(t, err, http.StatusConflict, "changed")

	_, err = store.Authorize(t.Context(), client, key, "local", noGrantCheck, write)
	requireHTTPError(t, err, http.StatusBadRequest, "not a remote server")
	require.Len(t, written, 2, "a refused grant was stored")

	// The stored status names grants, never their tokens.
	statusJSON, err := json.Marshal(storedVMCP(t, client, key).Status)
	require.NoError(t, err)
	require.NotContains(t, string(statusJSON), "token")
}

// TestPreconfiguredOAuthTarget checks which components of which vMCPs can hold a grant: a remote component of a shared vMCP
// that is not being deleted, whichever profiles grant it (TestEveryRemoteComponentCanBePreconfigured).
func TestPreconfiguredOAuthTarget(t *testing.T) {
	personal := grantTestVMCP()
	personal.Spec.UserID = "1"
	deleting := grantTestVMCP()
	deleting.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	for _, tt := range []struct {
		name      string
		vmcp      v1.VMCP
		component string
		code      int
		contains  string
	}{
		{
			name:      "a remote component",
			vmcp:      grantTestVMCP(),
			component: "dynamic",
		},
		{
			name:      "an unknown component",
			vmcp:      grantTestVMCP(),
			component: "missing",
			code:      http.StatusNotFound,
			contains:  "no component",
		},
		{
			name:      "a component without OAuth",
			vmcp:      grantTestVMCP(),
			component: "local",
			code:      http.StatusBadRequest,
			contains:  "not a remote server",
		},
		{
			name:      "a personal vMCP",
			vmcp:      personal,
			component: "static",
			code:      http.StatusBadRequest,
			contains:  "personal",
		},
		{
			name:      "a vMCP being deleted",
			vmcp:      deleting,
			component: "static",
			code:      http.StatusConflict,
			contains:  "being deleted",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			component, err := PreconfiguredOAuthTarget(&tt.vmcp, tt.component)
			if tt.code == 0 {
				require.NoError(t, err)
				require.Equal(t, tt.component, component.ID)
				return
			}
			requireHTTPError(t, err, tt.code, tt.contains)
		})
	}
}

// TestPreconfiguredOAuthAuthorizeDeletesUnpublishedGrant removes the component while its grant is being stored: the grant
// is refused, and leaves nothing behind, even though the controller deleted the component's recorded grants before it
// was stored.
func TestPreconfiguredOAuthAuthorizeDeletesUnpublishedGrant(t *testing.T) {
	client, key := newGrantTestStorage(t, grantTestVMCP())
	grants := newMemoryGrants()
	store := NewPreconfiguredOAuthStore(grants)
	var attempts int
	write := func(_ context.Context, owner, mcpID, grantID string) error {
		attempts++
		current := storedVMCP(t, client, key)
		current.Spec.Manifest.Components = current.Spec.Manifest.Components[1:]
		require.NoError(t, client.Update(t.Context(), current))
		// The controller deletes the removed component's recorded grants before this write stores the grant.
		current = storedVMCP(t, client, key)
		changed, err := store.DeleteRemovedComponentGrants(t.Context(), current)
		require.NoError(t, err)
		require.True(t, changed)
		require.NoError(t, client.Status().Update(t.Context(), current))
		grants.put(owner, mcpID, grantID, "https://static.example.com/mcp")
		return nil
	}

	_, err := store.Authorize(t.Context(), client, key, "static", noGrantCheck, write)
	requireHTTPError(t, err, http.StatusNotFound, "no component")
	require.Equal(t, 1, attempts)
	_, ok := grants.get(grantOwner(), grantMCPID("static"))
	require.False(t, ok, "the unpublished grant was left behind")
	require.Empty(t, storedVMCP(t, client, key).Status.PreconfiguredOAuth)
}

// TestPreconfiguredOAuthRevoke checks that revoking deletes the vMCP's grant for a component and its record, that it is
// allowed where authorizing is not, and that it deletes only the grants it listed: one authorized after it listed them
// is kept.
func TestPreconfiguredOAuthRevoke(t *testing.T) {
	client, key := newGrantTestStorage(t, grantTestVMCP())
	grants := newMemoryGrants()
	store := NewPreconfiguredOAuthStore(grants)
	write := func(_ context.Context, owner, mcpID, grantID string) error {
		grants.put(owner, mcpID, grantID, "https://"+strings.TrimPrefix(mcpID, grantVMCP+"/")+".example.com/mcp")
		return nil
	}
	for _, componentID := range []string{"static", "dynamic"} {
		_, err := store.Authorize(t.Context(), client, key, componentID, noGrantCheck, write)
		require.NoError(t, err)
	}

	updated, revoked, err := store.Revoke(t.Context(), client, key, "static")
	require.NoError(t, err)
	require.True(t, revoked)
	require.Empty(t, recordedGrantIDs(updated, "static"))
	_, ok := grants.get(grantOwner(), grantMCPID("static"))
	require.False(t, ok)
	// Another component's grant is kept.
	_, ok = grants.get(grantOwner(), grantMCPID("dynamic"))
	require.True(t, ok)

	_, revoked, err = store.Revoke(t.Context(), client, key, "static")
	require.NoError(t, err)
	require.False(t, revoked)

	// An administrator authorizes the grant again while a revocation is deleting it: the new grant has an ID the
	// revocation did not list, and is kept.
	grants.beforeDelete = func() {
		grants.beforeDelete = nil
		grants.put(grantOwner(), grantMCPID("dynamic"), "grant-after-listing", "https://dynamic.example.com/mcp")
	}
	_, revoked, err = store.Revoke(t.Context(), client, key, "dynamic")
	require.NoError(t, err)
	require.True(t, revoked)
	kept, ok := grants.get(grantOwner(), grantMCPID("dynamic"))
	require.True(t, ok)
	require.Equal(t, "grant-after-listing", kept.GrantID)

	// Revoking is allowed for a component that no agent is granted any more, and for one the vMCP no longer has.
	current := storedVMCP(t, client, key)
	current.Spec.Manifest.Profiles = []types.VMCPProfile{everyUserProfile(grantUsers)}
	current.Spec.Manifest.Components = current.Spec.Manifest.Components[2:]
	require.NoError(t, client.Update(t.Context(), current))
	_, revoked, err = store.Revoke(t.Context(), client, key, "dynamic")
	require.NoError(t, err)
	require.True(t, revoked)
	_, ok = grants.get(grantOwner(), grantMCPID("dynamic"))
	require.False(t, ok)

	_, _, err = store.Revoke(t.Context(), client, kclient.ObjectKey{Namespace: key.Namespace, Name: "vmcp1missing"}, "static")
	requireHTTPError(t, err, http.StatusNotFound, "not found")
}

// TestDeleteRemovedComponentGrants checks that the grants of a component removed from the vMCP are deleted by their
// recorded IDs, and their records with them; that other components' grants are kept; and that a grant written since,
// for a component of the same ID added back, has an ID the records do not hold, and is kept too.
func TestDeleteRemovedComponentGrants(t *testing.T) {
	vmcp := grantTestVMCP()
	vmcp.Status.PreconfiguredOAuth = []v1.VMCPPreconfiguredOAuthGrant{
		{ComponentID: "static", GrantIDs: []string{"grant-static"}},
		{ComponentID: "dynamic", GrantIDs: []string{"grant-dynamic", "grant-pending"}},
		{ComponentID: "removed", GrantIDs: []string{"grant-removed"}},
		{ComponentID: "readded", GrantIDs: []string{"grant-before-removal"}},
	}
	grants := newMemoryGrants()
	grants.put(grantOwner(), grantMCPID("static"), "grant-static", "https://static.example.com/mcp")
	grants.put(grantOwner(), grantMCPID("dynamic"), "grant-pending", "https://dynamic.example.com/mcp")
	grants.put(grantOwner(), grantMCPID("removed"), "grant-removed", "https://removed.example.com/mcp")
	grants.put(grantOwner(), grantMCPID("readded"), "grant-after-readding", "https://readded.example.com/mcp")
	store := NewPreconfiguredOAuthStore(grants)

	changed, err := store.DeleteRemovedComponentGrants(t.Context(), &vmcp)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []v1.VMCPPreconfiguredOAuthGrant{
		{ComponentID: "static", GrantIDs: []string{"grant-static"}},
		{ComponentID: "dynamic", GrantIDs: []string{"grant-dynamic", "grant-pending"}},
	}, vmcp.Status.PreconfiguredOAuth)
	for _, tt := range []struct {
		componentID string
		kept        bool
	}{
		{
			componentID: "static",
			kept:        true,
		},
		{
			componentID: "dynamic",
			kept:        true,
		},
		{
			componentID: "removed",
		},
		{
			componentID: "readded",
			kept:        true,
		},
	} {
		_, ok := grants.get(grantOwner(), grantMCPID(tt.componentID))
		require.Equal(t, tt.kept, ok, "grant for %s", tt.componentID)
	}

	changed, err = store.DeleteRemovedComponentGrants(t.Context(), &vmcp)
	require.NoError(t, err)
	require.False(t, changed)
}

// TestDeleteVMCPGrants checks that deleting a vMCP deletes every one of its grants, recorded or not, with one statement,
// that it can be repeated, and that it leaves other vMCPs' grants and every user's and agent's token alone.
func TestDeleteVMCPGrants(t *testing.T) {
	vmcp := grantTestVMCP()
	vmcp.Status.PreconfiguredOAuth = []v1.VMCPPreconfiguredOAuthGrant{{ComponentID: "static", GrantIDs: []string{"a"}}}
	grants := newMemoryGrants()
	grants.put(grantOwner(), grantMCPID("static"), "a", "https://static.example.com/mcp")
	grants.put(grantOwner(), grantMCPID("dynamic"), "b", "https://dynamic.example.com/mcp")
	other := principal.PreconfiguredOAuthOwner("vmcp1other")
	grants.put(other, PreconfiguredOAuthMCPID("vmcp1other", "static"), "c", "https://static.example.com/mcp")
	grants.put("7", grantMCPID("static"), "d", "https://static.example.com/mcp")
	grants.put("agent:7", "msi1connection", "e", "https://static.example.com/mcp")
	store := NewPreconfiguredOAuthStore(grants)

	for range 2 {
		require.NoError(t, store.DeleteVMCPGrants(t.Context(), vmcp))
	}
	for _, component := range []string{"static", "dynamic"} {
		_, ok := grants.get(grantOwner(), grantMCPID(component))
		require.False(t, ok, "the vMCP's grant for %s was kept", component)
	}
	for _, key := range []gatewaytypes.MCPOAuthTokenKey{
		{UserID: other, MCPID: PreconfiguredOAuthMCPID("vmcp1other", "static")},
		{UserID: "7", MCPID: grantMCPID("static")},
		{UserID: "agent:7", MCPID: "msi1connection"},
	} {
		_, ok := grants.get(key.UserID, key.MCPID)
		require.True(t, ok, "the token of %s for %s was deleted with the vMCP's grants", key.UserID, key.MCPID)
	}
}

// TestGrantRemovalsAreRecorded checks that the grants the vMCP controller deletes with a removed component or a deleted
// vMCP are reported, once each, and only for grants that were stored: a component whose grant was recorded but never
// stored, and a deletion that is repeated, report nothing.
func TestGrantRemovalsAreRecorded(t *testing.T) {
	type removal struct{ vmcpID, componentID, reason string }
	var removed []removal
	record := func(_ context.Context, vmcpID, componentID, reason string) {
		removed = append(removed, removal{vmcpID, componentID, reason})
	}

	grants := newMemoryGrants()
	store := NewPreconfiguredOAuthStore(grants)
	store.SetGrantRemovalRecorder(record)

	vmcp := grantTestVMCP()
	vmcp.Status.PreconfiguredOAuth = []v1.VMCPPreconfiguredOAuthGrant{
		{ComponentID: "removed", GrantIDs: []string{"grant-removed"}},
		{ComponentID: "never-stored", GrantIDs: []string{"grant-never"}},
		{ComponentID: "static", GrantIDs: []string{"grant-static"}},
	}
	grants.put(grantOwner(), grantMCPID("removed"), "grant-removed", "https://removed.example.com/mcp")
	grants.put(grantOwner(), grantMCPID("static"), "grant-static", "https://static.example.com/mcp")
	changed, err := store.DeleteRemovedComponentGrants(t.Context(), &vmcp)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []removal{{grantVMCP, "removed", "component removed"}}, removed)

	// Deleting the vMCP reports each grant it held, once, and a deletion repeated after it succeeded reports nothing.
	removed = nil
	grants.put(grantOwner(), grantMCPID("dynamic"), "grant-dynamic", "https://dynamic.example.com/mcp")
	for range 2 {
		require.NoError(t, store.DeleteVMCPGrants(t.Context(), vmcp))
	}
	slices.SortFunc(removed, func(a, b removal) int { return strings.Compare(a.componentID, b.componentID) })
	require.Equal(t, []removal{{grantVMCP, "dynamic", "vMCP deleted"}, {grantVMCP, "static", "vMCP deleted"}}, removed)

	// Without a recorder, deletions work and nothing is read for the audit.
	quiet := newMemoryGrants()
	quiet.put(grantOwner(), grantMCPID("static"), "a", "https://static.example.com/mcp")
	require.NoError(t, NewPreconfiguredOAuthStore(quiet).DeleteVMCPGrants(t.Context(), grantTestVMCP()))
	require.Zero(t, quiet.queries)
	_, ok := quiet.get(grantOwner(), grantMCPID("static"))
	require.False(t, ok)
}

// TestComponentOAuthGrantStatus checks the status of the vMCP's grants: who authorized each, when, for which target and
// scopes, whether it works, and whether the component needs OAuth, with no token, and only the hostname of a URL that
// the vMCP's fixed configuration sets.
func TestComponentOAuthGrantStatus(t *testing.T) {
	vmcp := grantTestVMCP()
	grants := newMemoryGrants()
	grants.put(grantOwner(), grantMCPID("static"), "a", "https://static.example.com/mcp")
	grants.put(grantOwner(), grantMCPID("dynamic"), "b", "https://dynamic.example.com/mcp")
	grants.invalidate(grantOwner(), grantMCPID("dynamic"))
	store := NewPreconfiguredOAuthStore(grants)

	selected, err := store.SelectGrants(t.Context(), vmcp, vmcp.Spec.Manifest.Components)
	require.NoError(t, err)
	static := selected[0].Status(vmcp.Name, vmcp.Spec.Manifest.Components[0])
	require.NotNil(t, static.AuthorizedAt)
	static.AuthorizedAt = nil
	require.Equal(t, types.VMCPPreconfiguredOAuth{
		VMCPID:       vmcp.Name,
		ComponentID:  "static",
		State:        types.VMCPPreconfiguredOAuthAuthorized,
		AuthorizedBy: "1",
		Target:       "https://static.example.com/mcp",
		Scopes:       []string{"read", "write"},
	}, static)
	dynamic := selected[1].Status(vmcp.Name, vmcp.Spec.Manifest.Components[1])
	require.Equal(t, types.VMCPPreconfiguredOAuthInvalid, dynamic.State)
	require.Equal(t, "refused", dynamic.InvalidReason)
	require.Equal(t, types.VMCPPreconfiguredOAuth{VMCPID: vmcp.Name, ComponentID: "local", State: types.VMCPPreconfiguredOAuthNotRequired}, selected[2].Status(vmcp.Name, vmcp.Spec.Manifest.Components[2]))

	// Once the vMCP holds no grant for it, nothing shows that a component without static OAuth needs OAuth until a probe
	// finds it does; one with static OAuth needs authorization.
	require.NoError(t, grants.DeleteMCPOAuthGrants(t.Context(), grantOwner(), grantMCPID("dynamic"), []string{"b"}))
	require.NoError(t, grants.DeleteMCPOAuthGrants(t.Context(), grantOwner(), grantMCPID("static"), []string{"a"}))
	selected, err = store.SelectGrants(t.Context(), vmcp, vmcp.Spec.Manifest.Components)
	require.NoError(t, err)
	require.Equal(t, types.VMCPPreconfiguredOAuth{VMCPID: vmcp.Name, ComponentID: "static", State: types.VMCPPreconfiguredOAuthNeedsAuthorization}, selected[0].Status(vmcp.Name, vmcp.Spec.Manifest.Components[0]))
	require.Equal(t, types.VMCPPreconfiguredOAuth{VMCPID: vmcp.Name, ComponentID: "dynamic", State: types.VMCPPreconfiguredOAuthNotRequired}, selected[1].Status(vmcp.Name, vmcp.Spec.Manifest.Components[1]))
	recordProbe(&vmcp, vmcp.Spec.Manifest.Components[1], v1.VMCPComponentOAuthRequired)
	status, err := store.Status(t.Context(), vmcp, "dynamic")
	require.NoError(t, err)
	require.Equal(t, types.VMCPPreconfiguredOAuthNeedsAuthorization, status.State)

	// A grant for a URL the vMCP's fixed configuration sets is reported by its hostname only.
	const configured = "https://user:pw@tenant.example.com:8443/secret/path?token=secret"
	remote := fixedURL(hostnameComponent("tenant"))
	tenant := readinessVMCP([]types.VMCPComponent{remote}, agentNamingProfile(grantAgent7, "7"))
	tenantGrants := newMemoryGrants()
	tenantGrants.put(principal.PreconfiguredOAuthOwner(tenant.Name), PreconfiguredOAuthMCPID(tenant.Name, "tenant"), "c", configured)
	tenantStore := NewPreconfiguredOAuthStore(tenantGrants)
	tenantStore.SetComponentTargets(fakeTargets{url: configured})
	readiness, err := readinessWith(t, tenantStore, tenant, agentInfo(7, nil))
	require.NoError(t, err)
	require.Empty(t, readiness.Reasons)
	status, err = tenantStore.Status(t.Context(), tenant, "tenant")
	require.NoError(t, err)
	require.Equal(t, types.VMCPPreconfiguredOAuthAuthorized, status.State)
	require.Empty(t, status.Target)
	require.Equal(t, "tenant.example.com", status.TargetHostname)
	statusJSON, err := json.Marshal(status)
	require.NoError(t, err)
	for _, secret := range []string{"secret/path", "token=secret", "user:pw", "access", "refresh"} {
		require.NotContains(t, string(statusJSON), secret)
	}
}

// TestCheckGrantTarget checks the comparison of a working grant with the URL agents' connections reach: a grant for that
// URL works, one for another URL, or for a URL the component's hostname no longer allows, is invalid, and one whose URL
// cannot be resolved for want of a value is left as it is, for configuration readiness to report.
func TestCheckGrantTarget(t *testing.T) {
	const authorized = "https://tenant.example.com/mcp"
	component := fixedURL(hostnameComponent("tenant"))
	vmcp := readinessVMCP([]types.VMCPComponent{component}, agentNamingProfile(grantAgent7, "7"))
	refused := types.NewErrBadRequest("the configured URL does not have the hostname tenant.example.com")
	for _, tt := range []struct {
		name        string
		targets     ComponentTargetResolver
		wantState   string
		wantReason  string
		wantFailure bool
	}{
		{
			name:      "the URL the grant was authorized for",
			targets:   fakeTargets{url: authorized},
			wantState: types.VMCPPreconfiguredOAuthAuthorized,
		},
		{
			name:       "another URL",
			targets:    fakeTargets{url: "https://other.example.com/mcp"},
			wantState:  types.VMCPPreconfiguredOAuthInvalid,
			wantReason: GrantTargetMismatch,
		},
		{
			name:       "a URL the component refuses",
			targets:    fakeTargets{refused: refused},
			wantState:  types.VMCPPreconfiguredOAuthInvalid,
			wantReason: refused.Error(),
		},
		{
			name:      "a URL that lacks a value",
			targets:   fakeTargets{missing: []string{"__url"}},
			wantState: types.VMCPPreconfiguredOAuthAuthorized,
		},
		{
			name:      "no resolver",
			wantState: types.VMCPPreconfiguredOAuthAuthorized,
		},
		{
			name:        "a resolver that fails",
			targets:     fakeTargets{err: errors.New("credential store unavailable")},
			wantFailure: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			grants := newMemoryGrants()
			grants.put(principal.PreconfiguredOAuthOwner(vmcp.Name), PreconfiguredOAuthMCPID(vmcp.Name, "tenant"), "grant", authorized)
			store := NewPreconfiguredOAuthStore(grants)
			if tt.targets != nil {
				store.SetComponentTargets(tt.targets)
			}
			components := ComponentsForInstance(vmcp, v1.VMCPInstance{})
			selected, err := store.SelectGrants(t.Context(), vmcp, components)
			require.NoError(t, err)
			err = store.checkGrantTarget(t.Context(), vmcp, components[0], &selected[0])
			if tt.wantFailure {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.wantState, selected[0].State)
			require.Equal(t, tt.wantReason, selected[0].InvalidReason)
		})
	}
}

// TestPreconfiguredOAuthOwnerIsNeverACaller checks that a caller whose UID is a vMCP's grant owner, however it presents
// itself, matches no profile, not even the one that names every user, owns no instance, and is not an agent, so it can
// never reach a vMCP, and so never the grants, as anyone.
func TestPreconfiguredOAuthOwnerIsNeverACaller(t *testing.T) {
	vmcp := grantTestVMCP()
	owner := grantOwner()
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
		_, err = agentReadiness(t, vmcp, caller)
		require.Error(t, err, "kind %q", kind)
	}
}

// TestGrantTargetForResponse checks that only a URL the catalog fixes is returned in full, and that any other URL is
// reduced to its hostname.
func TestGrantTargetForResponse(t *testing.T) {
	remote := func(config types.RemoteCatalogConfig) types.VMCPComponent {
		return types.VMCPComponent{CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Runtime: types.RuntimeRemote, RemoteConfig: &config}}}
	}
	const configured = "https://user:pw@tenant.example.com:8443/secret/path?token=secret"
	for _, tt := range []struct {
		name         string
		component    types.VMCPComponent
		grantURL     string
		wantTarget   string
		wantHostname string
	}{
		{
			name:       "a fixed URL",
			component:  remote(types.RemoteCatalogConfig{FixedURL: "https://fixed.example.com/mcp"}),
			grantURL:   "https://fixed.example.com/mcp",
			wantTarget: "https://fixed.example.com/mcp",
		},
		{
			name:         "a hostname-constrained URL",
			component:    remote(types.RemoteCatalogConfig{Hostname: "tenant.example.com"}),
			grantURL:     configured,
			wantHostname: "tenant.example.com",
		},
		{
			name:         "a URL template",
			component:    remote(types.RemoteCatalogConfig{URLTemplate: "https://${TENANT}.example.com/mcp"}),
			grantURL:     configured,
			wantHostname: "tenant.example.com",
		},
		{
			name:      "a URL that cannot be parsed",
			component: remote(types.RemoteCatalogConfig{URLTemplate: "${URL}"}),
			grantURL:  "://secret",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target, hostname := GrantTargetForResponse(tt.component, tt.grantURL)
			if target != tt.wantTarget || hostname != tt.wantHostname {
				t.Fatalf("GrantTargetForResponse() = (%q, %q), want (%q, %q)", target, hostname, tt.wantTarget, tt.wantHostname)
			}
		})
	}
}

// TestRedactURL checks that a configured URL, and the parts of it an error repeats, are removed from error text.
func TestRedactURL(t *testing.T) {
	const configured = "https://user:pw@tenant.example.com:8443/secret/path?token=secret"
	for _, text := range []string{
		`Get "` + configured + `": dial tcp: lookup failed`,
		"MCP server URL must not be a localhost URL: " + configured,
		`parse "https://user:pw@tenant.example.com:8443/secret/path": no such host; query token=secret`,
	} {
		redacted := RedactURL(text, configured)
		for _, secret := range []string{"secret/path", "token=secret", "user:pw", "https://tenant.example.com:8443"} {
			if strings.Contains(redacted, secret) {
				t.Errorf("RedactURL(%q) = %q, still has %q", text, redacted, secret)
			}
		}
	}
	if got := RedactURL("unchanged", ""); got != "unchanged" {
		t.Errorf("RedactURL without a URL = %q", got)
	}
}
