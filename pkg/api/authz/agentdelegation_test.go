package authz

import (
	"crypto/sha256"
	"encoding/base64"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/principal"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// browserUser returns the principal of a user's browser session, with role.
func browserUser(id string, role types.Role) user.Info {
	extra := map[string][]string{
		"auth_provider_name":      {"okta-auth-provider"},
		"auth_provider_namespace": {system.DefaultNamespace},
	}
	principal.RecordPrincipal(extra, principal.KindUser, types.UserStatusActive)
	return &user.DefaultInfo{Name: "user" + id, UID: id, Groups: role.Groups(), Extra: extra}
}

func withUserExtra(u user.Info, name string, extra map[string][]string) user.Info {
	merged := maps.Clone(u.GetExtra())
	maps.Copy(merged, extra)
	return &user.DefaultInfo{Name: name, UID: u.GetUID(), Groups: u.GetGroups(), Extra: merged}
}

// TestAgentDelegationApprovalRoutesNeedABrowserSession checks that only a user's browser session reads, approves or
// cancels an agent's request, and approves or cancels only from Obot's own pages.
func TestAgentDelegationApprovalRoutesNeedABrowserSession(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	authorizer := NewAuthorizer(nil, storage, storage, false, nil, nil, nil)
	routes := []agentTestRoute{
		{
			method: http.MethodGet,
			path:   "/api/agent-delegation-requests/adr1x",
		},
		{
			method: http.MethodPost,
			path:   "/api/agent-delegation-requests/adr1x/approve",
		},
		{
			method: http.MethodPost,
			path:   "/api/agent-delegation-requests/adr1x/cancel",
		},
	}
	alice := browserUser("5", types.RoleBasic)

	for _, tt := range []struct {
		name    string
		user    user.Info
		headers map[string]string
		reached bool
	}{
		{
			name:    "a user's browser, from Obot's page",
			user:    alice,
			headers: map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://example.com"},
			reached: true,
		},
		{
			name:    "an administrator's browser",
			user:    browserUser("1", types.RoleAdmin),
			reached: true,
		},
		{
			name:    "a client that is not a browser",
			user:    alice,
			reached: true,
		},
		{
			name: "the owner with authentication off",
			user: func() user.Info {
				extra := map[string][]string{"obot_groups": types.RoleOwner.Groups()}
				principal.RecordPrincipal(extra, principal.KindUser, types.UserStatusActive)
				return &user.DefaultInfo{Name: "nobody", UID: "1", Groups: (types.RoleOwner | types.RoleAuditor).Groups(), Extra: extra}
			}(),
			reached: true,
		},
		{
			name: "a user's API key",
			user: withUserExtra(alice, "user5", map[string][]string{
				principal.APIKeyIDExtra:   {"9"},
				principal.APIKeyNameExtra: {"cli"},
			}),
		},
		{
			name:    "a user named bootstrap",
			user:    withUserExtra(alice, system.BootstrapName, nil),
			reached: true,
		},
		{
			name: "the bootstrap login",
			user: withUserExtra(alice, system.BootstrapName, map[string][]string{"auth_provider_name": {system.BootstrapName}, "auth_provider_namespace": nil}),
		},
		{
			name: "the bootstrap auth provider",
			user: withUserExtra(alice, "user5", map[string][]string{"auth_provider_name": {system.BootstrapName}}),
		},
		{
			name: "a caller that records no principal",
			user: &user.DefaultInfo{Name: "user5", UID: "5", Groups: types.RoleBasic.Groups()},
		},
		{
			name: "an agent's key",
			user: agentKeyPrincipal(gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true}),
		},
		{
			name: "unauthenticated",
			user: &user.DefaultInfo{Name: "anonymous", Groups: []string{UnauthenticatedGroup}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, route := range routes {
				req := httptest.NewRequest(route.method, route.path, nil)
				for name, value := range tt.headers {
					req.Header.Set(name, value)
				}
				require.Equal(t, tt.reached, authorizer.Authorize(req, tt.user), "%s %s", route.method, route.path)
			}
		})
	}

	// Another site's page reads nothing it could not before, but approves and cancels nothing.
	for _, headers := range []map[string]string{
		{"Sec-Fetch-Site": "cross-site"},
		{"Sec-Fetch-Site": "same-site"},
		{"Origin": "https://evil.example.com"},
	} {
		for _, route := range routes {
			req := httptest.NewRequest(route.method, route.path, nil)
			for name, value := range headers {
				req.Header.Set(name, value)
			}
			require.Equal(t, route.method == http.MethodGet, authorizer.Authorize(req, alice), "%v: %s %s", headers, route.method, route.path)
		}
	}
}

// newGatewayTestAuthorizer returns an authorizer with an empty gateway database, for the resource checks that read it,
// such as checkAgentDelegation, and the services the database is in.
func newGatewayTestAuthorizer(t *testing.T) (*Authorizer, *gatewayclient.Client, *storageservices.Services) {
	t.Helper()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	gateway := gatewayclient.New(t.Context(), database, storage, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	return NewAuthorizer(gateway, storage, storage, false, nil, nil, nil), gateway, services
}

// TestAgentDelegationsAreTheirUsers checks that a user reaches only their own delegation under /api/me, whatever their
// role, and lists their own.
func TestAgentDelegationsAreTheirUsers(t *testing.T) {
	ctx := t.Context()
	authorizer, gateway, services := newGatewayTestAuthorizer(t)

	var users []*gatewaytypes.User
	for _, name := range []string{"alice", "bob", "root"} {
		u := &gatewaytypes.User{Username: name, HashedUsername: hash.String(name), Role: types.RoleBasic}
		require.NoError(t, services.DB.DB.Create(u).Error)
		users = append(users, u)
	}
	agent, err := gateway.CreateAgent(ctx, gatewaytypes.Agent{
		DisplayName:        "assistant",
		Profiles:           []types.AgentProfile{{Name: "everyone", Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}}}},
		AllowedReturnHosts: []string{"agent.example.com"},
	})
	require.NoError(t, err)
	digest := sha256.Sum256([]byte(strings.Repeat("s", 43)))
	request, err := gateway.CreateAgentDelegationRequest(ctx, gatewayclient.AgentDelegationRequestInput{
		AgentID: agent.ID, APIKeyID: 1, ReturnURL: "https://agent.example.com/callback", SecretHash: base64.RawURLEncoding.EncodeToString(digest[:]),
	})
	require.NoError(t, err)
	approval, _, err := gateway.ApproveAgentDelegationRequest(ctx, request.ID, subjects.Caller{Principal: principal.User(users[0].ID)})
	require.NoError(t, err)

	alice := browserUser(principal.User(users[0].ID).ID, types.RoleBasic)
	bob := browserUser(principal.User(users[1].ID).ID, types.RoleBasic)
	admin := browserUser(principal.User(users[2].ID).ID, types.RoleAdmin|types.RoleOwner)
	revoke := func(id string) *http.Request {
		return httptest.NewRequest(http.MethodDelete, "/api/me/agent-delegations/"+id, nil)
	}

	require.True(t, authorizer.Authorize(revoke(approval.Delegation.ID), alice))
	require.False(t, authorizer.Authorize(revoke(approval.Delegation.ID), bob))
	require.False(t, authorizer.Authorize(revoke(approval.Delegation.ID), admin), "an administrator revokes through the agent's routes")
	require.False(t, authorizer.Authorize(revoke("adl1missing"), alice))
	require.True(t, authorizer.Authorize(httptest.NewRequest(http.MethodDelete, "/api/agents/"+principal.Agent(agent.ID).ID+"/delegations/"+approval.Delegation.ID, nil), admin))
	require.False(t, authorizer.Authorize(httptest.NewRequest(http.MethodDelete, "/api/agents/"+principal.Agent(agent.ID).ID+"/delegations/"+approval.Delegation.ID, nil), alice))
	for _, u := range []user.Info{alice, bob, admin} {
		require.True(t, authorizer.Authorize(httptest.NewRequest(http.MethodGet, "/api/me/agent-delegations", nil), u))
	}
	// Agents reach none of them.
	agentKey := agentKeyPrincipal(gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true})
	require.False(t, authorizer.Authorize(revoke(approval.Delegation.ID), agentKey))
	require.False(t, authorizer.Authorize(httptest.NewRequest(http.MethodGet, "/api/me/agent-delegations", nil), agentKey))
}
