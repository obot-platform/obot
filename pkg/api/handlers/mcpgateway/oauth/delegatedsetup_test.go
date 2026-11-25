package oauth

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// delegatedSetupReturnPath is the page that starts the authorizations in these tests.
	delegatedSetupReturnPath = "/agents/approve/adr1test"
)

// delegatedSetupFixture is the agent grant fixture's vMCP, with the instance of what user 9 set up through agent 7, and
// the pair's connection to the remote component's shared server, as the controllers create them.
type delegatedSetupFixture struct {
	*agentGrantFixture
	vmcp       *v1.VMCP
	pair       *v1.VMCPInstance
	connection string
}

func newDelegatedSetupFixture(t *testing.T) *delegatedSetupFixture {
	t.Helper()
	f := newAgentGrantFixture(t)
	var vmcp v1.VMCP
	require.NoError(t, f.storage.Get(t.Context(), f.key, &vmcp))
	component := vmcpconfig.ComponentsForInstance(vmcp, v1.VMCPInstance{})[0]

	shared := &v1.MCPServer{
		Name:      "ms1agentgrantsremote",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			VMCPID:                    vmcp.Name,
			VMCPComponentID:           component.ID,
			MCPServerCatalogEntryName: component.MCPServerCatalogEntryID,
			Manifest: types.MCPServerManifest{
				Name:         component.Name,
				Runtime:      types.RuntimeRemote,
				RemoteConfig: &types.RemoteRuntimeConfig{URL: f.provider.mcpURL()},
			},
		},
	}
	require.NoError(t, f.storage.Create(t.Context(), shared))
	pair := &v1.VMCPInstance{
		Name:      "vmcpi1pair",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			UserID:   principal.DelegatedOwner{AgentID: 7, UserID: 9}.String(),
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		},
	}
	pair.Status.ConfigurationCheckHash = vmcpconfig.ConfigurationCheckHash(vmcpconfig.PairInstanceComponents(vmcpconfig.ComponentsForInstance(vmcp, *pair)), "")
	require.NoError(t, f.storage.Create(t.Context(), pair))
	config := vmcpconfig.ConnectionConfiguration(component)
	connection := &v1.MCPServerInstance{
		Name:      vmcpconfig.ComponentConnectionName(pair.Name, component.ID),
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerInstanceSpec{
			UserID:          pair.Spec.UserID,
			MCPServerName:   shared.Name,
			VMCPInstanceID:  pair.Name,
			VMCPComponentID: component.ID,
			Config:          config,
		},
	}
	connection.Status.VMCPConfigurationHash = vmcpconfig.ConnectionConfigurationHash(config, *pair)
	require.NoError(t, f.storage.Create(t.Context(), connection))
	return &delegatedSetupFixture{agentGrantFixture: f, vmcp: &vmcp, pair: pair, connection: connection.Name}
}

// signedInUser returns the principal of the signed-in user with id.
func signedInUser(id string) kuser.Info {
	return principaltest.AsUser(&kuser.DefaultInfo{Name: "user-" + id, UID: id, Groups: []string{types.GroupAuthenticated}})
}

// startPair starts the authorization of the pair's account for the remote component as caller, and returns the state
// of its pending attempt.
func (f *delegatedSetupFixture) startPair(t *testing.T, caller kuser.Info) (string, error) {
	t.Helper()
	req, _ := f.request(caller, http.MethodPost, "/api/agent-delegation-requests/adr1test/setup/vmcps/"+f.vmcp.Name+"/components/"+agentGrantRemote+"/oauth")
	authorization, err := f.factory.StartDelegatedSetupOAuth(req, f.vmcp, f.pair, agentGrantRemote, delegatedSetupReturnPath)
	if err != nil {
		return "", err
	}
	require.False(t, authorization.LocalhostCallback)
	parsed, err := url.Parse(authorization.AuthURL)
	require.NoError(t, err)
	require.Equal(t, f.provider.server.URL+"/authorize", parsed.Scheme+"://"+parsed.Host+parsed.Path)
	require.Equal(t, system.MCPOAuthCallbackURL("http://obot.example"), parsed.Query().Get("redirect_uri"))
	return parsed.Query().Get("state"), nil
}

// pairToken returns the access token stored for an owner and the pair's connection, if any.
func (f *delegatedSetupFixture) pairToken(t *testing.T, owner string) (string, bool) {
	t.Helper()
	token, err := f.gateway.GetMCPOAuthToken(t.Context(), owner, f.connection, f.provider.mcpURL())
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", false
	}
	require.NoError(t, err)
	return token.AccessToken, true
}

// TestDelegatedSetupOAuth checks the authorization of a user's upstream account through an agent: only the pair's user
// can start it, its callback is refused to anyone else, an administrator included, without consuming it, and once the
// user completes it, the account is stored for the pair and the pair's connection alone, and the browser goes back to
// the page that started it. The attempt cannot be used again.
func TestDelegatedSetupOAuth(t *testing.T) {
	f := newDelegatedSetupFixture(t)
	pairOwner := f.pair.Spec.UserID

	// Only the user of the pair starts it: not another user, an administrator, or the agent.
	for _, caller := range []kuser.Info{signedInUser("10"), administrator("1"), agentGrantAgent(7)} {
		_, err := f.startPair(t, caller)
		requireErrHTTP(t, err, http.StatusForbidden)
	}
	f.requireNoPending(t)

	state, err := f.startPair(t, signedInUser("9"))
	require.NoError(t, err)
	var pending gatewaytypes.MCPOAuthPendingState
	require.NoError(t, f.db.First(&pending).Error)
	require.Equal(t, pairOwner, pending.UserID)
	require.Equal(t, f.connection, pending.MCPID)
	require.Equal(t, "9", pending.GrantInitiatorUserID)
	require.Equal(t, delegatedSetupReturnPath, pending.ReturnPath)

	// Anyone else's callback is refused, without consuming the attempt.
	for _, caller := range []kuser.Info{signedInUser("10"), administrator("1"), agentGrantAgent(7)} {
		_, err := f.callback(caller, state)
		requireErrHTTP(t, err, http.StatusForbidden)
	}
	_, held := f.pairToken(t, pairOwner)
	require.False(t, held)

	recorder, err := f.callback(signedInUser("9"), state)
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, delegatedSetupReturnPath, recorder.Header().Get("Location"))
	token, held := f.pairToken(t, pairOwner)
	require.True(t, held)
	require.Equal(t, agentGrantAccess, token)
	// The account is the pair's alone: the user has none of their own for the connection, and neither has the agent.
	for _, owner := range []string{"9", "agent:7"} {
		_, held := f.pairToken(t, owner)
		require.False(t, held, "the account was stored for %s", owner)
	}

	_, err = f.callback(signedInUser("9"), state)
	require.Error(t, err, "an attempt was used twice")
}

// TestDelegatedSetupOAuthForRemovedSetup checks that an account authorized after the setup it was for was removed, as
// revoking the delegation removes it, is never stored.
func TestDelegatedSetupOAuthForRemovedSetup(t *testing.T) {
	f := newDelegatedSetupFixture(t)
	state, err := f.startPair(t, signedInUser("9"))
	require.NoError(t, err)

	require.NoError(t, f.storage.Delete(t.Context(), &v1.MCPServerInstance{Name: f.connection, Namespace: system.DefaultNamespace}))
	_, err = f.callback(signedInUser("9"), state)
	requireErrHTTP(t, err, http.StatusConflict)
	_, held := f.pairToken(t, f.pair.Spec.UserID)
	require.False(t, held)
	f.requireNoPending(t)
}

// TestDelegatedSetupOAuthExpires checks that an attempt left too long is refused, and removed.
func TestDelegatedSetupOAuthExpires(t *testing.T) {
	f := newDelegatedSetupFixture(t)
	state, err := f.startPair(t, signedInUser("9"))
	require.NoError(t, err)
	require.NoError(t, f.db.Model(&gatewaytypes.MCPOAuthPendingState{}).Where("1 = 1").Update("created_at", time.Now().Add(-time.Hour)).Error)

	_, err = f.callback(signedInUser("9"), state)
	requireErrHTTP(t, err, http.StatusBadRequest)
	f.requireNoPending(t)
}
