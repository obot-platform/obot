package authz

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

const (
	agentTestVMCP         = "vmcp1shared"
	agentTestSkill        = "sk1agent"
	agentTestSystemServer = "sms1hooks"
)

var (
	// agentReachableRoutes are every route an agent's principal can reach with all the scopes of an agent's key: the
	// LLM proxy, the skill GET routes, MCP connections, and the API key webhook.
	agentReachableRoutes = []agentTestRoute{
		{
			method: http.MethodPost,
			path:   "/api/llm-proxy/v1/chat/completions",
		},
		{
			method: http.MethodPost,
			path:   "/api/llm-proxy/v1/messages",
		},
		{
			method: http.MethodGet,
			path:   "/api/llm-proxy/v1/models",
		},
		{
			method: http.MethodGet,
			path:   "/api/skills",
		},
		{
			method: http.MethodGet,
			path:   "/api/skills/" + agentTestSkill,
		},
		{
			method: http.MethodGet,
			path:   "/api/skills/" + agentTestSkill + "/download",
		},
		{
			method: http.MethodGet,
			path:   "/api/skills/" + agentTestSkill + "/preview",
		},
		{
			method: http.MethodPost,
			path:   "/mcp-connect/" + agentTestVMCP,
		},
		{
			method: http.MethodGet,
			path:   "/mcp-connect/" + agentTestVMCP,
		},
		{
			method: http.MethodDelete,
			path:   "/mcp-connect/" + agentTestVMCP,
		},
		{
			method: http.MethodPost,
			path:   "/api/api-keys/auth",
		},
	}

	// agentUnreachableRoutes are routes no agent reaches: administration, user, device and API key management, the
	// routes open to any caller or to any authenticated one, the UI, and every route whose handler reads
	// api.Context.UserID without rejecting 0 (see the agent-identities call-site checklist).
	agentUnreachableRoutes = []agentTestRoute{
		// The caller's own user.
		{
			method: http.MethodGet,
			path:   "/api/me",
		},
		{
			method: http.MethodPatch,
			path:   "/api/me",
		},
		{
			method: http.MethodDelete,
			path:   "/api/me",
		},
		{
			method: http.MethodPost,
			path:   "/api/logout-all",
		},
		{
			method: http.MethodPost,
			path:   "/api/local-auth/change-password",
		},
		// The admin API of agents, which no agent reaches, not even for itself.
		{
			method: http.MethodGet,
			path:   "/api/agents",
		},
		{
			method: http.MethodPost,
			path:   "/api/agents",
		},
		{
			method: http.MethodGet,
			path:   "/api/agents/7",
		},
		{
			method: http.MethodPut,
			path:   "/api/agents/7",
		},
		{
			method: http.MethodDelete,
			path:   "/api/agents/7",
		},
		{
			method: http.MethodPost,
			path:   "/api/agents/7/enable",
		},
		{
			method: http.MethodGet,
			path:   "/api/agents/7/api-keys",
		},
		{
			method: http.MethodPost,
			path:   "/api/agents/7/api-keys",
		},
		// API key management, the caller's own and every user's.
		{
			method: http.MethodPost,
			path:   "/api/api-keys",
		},
		{
			method: http.MethodGet,
			path:   "/api/api-keys",
		},
		{
			method: http.MethodGet,
			path:   "/api/api-keys/1",
		},
		{
			method: http.MethodDelete,
			path:   "/api/api-keys/1",
		},
		{
			method: http.MethodGet,
			path:   "/api/admin-api-keys",
		},
		{
			method: http.MethodGet,
			path:   "/api/admin-api-keys/1",
		},
		{
			method: http.MethodDelete,
			path:   "/api/admin-api-keys/1",
		},
		// Users, including the user with the agent's numeric ID, and the agent's own encoded ID.
		{
			method: http.MethodGet,
			path:   "/api/users",
		},
		{
			method: http.MethodGet,
			path:   "/api/users/7",
		},
		{
			method: http.MethodGet,
			path:   "/api/users/agent:7",
		},
		{
			method: http.MethodPatch,
			path:   "/api/users/7",
		},
		{
			method: http.MethodPost,
			path:   "/api/users/7/enable",
		},
		{
			method: http.MethodGet,
			path:   "/api/groups",
		},
		{
			method: http.MethodGet,
			path:   "/api/tokens",
		},
		{
			method: http.MethodDelete,
			path:   "/api/tokens/1",
		},
		// Administration.
		{
			method: http.MethodGet,
			path:   "/api/model-providers",
		},
		{
			method: http.MethodPost,
			path:   "/api/mcp-catalogs",
		},
		{
			method: http.MethodGet,
			path:   "/api/mcp-audit-logs",
		},
		{
			method: http.MethodPost,
			path:   "/api/auth-providers/github/stage",
		},
		{
			method: http.MethodPost,
			path:   "/api/scim-connections",
		},
		{
			method: http.MethodPost,
			path:   "/api/mdm/configurations",
		},
		{
			method: http.MethodPost,
			path:   "/api/mdm/configurations/mdm1/enrollment-keys",
		},
		{
			method: http.MethodGet,
			path:   "/api/llm-gateway/settings",
		},
		// MCP management, as opposed to connecting.
		{
			method: http.MethodGet,
			path:   "/api/mcp-servers",
		},
		{
			method: http.MethodGet,
			path:   "/api/vmcps",
		},
		{
			method: http.MethodPost,
			path:   "/api/vmcps",
		},
		{
			method: http.MethodGet,
			path:   "/api/vmcps/" + agentTestVMCP,
		},
		{
			method: http.MethodGet,
			path:   "/api/vmcps/" + agentTestVMCP + "/tools",
		},
		{
			method: http.MethodGet,
			path:   "/api/vmcp-instances",
		},
		{
			method: http.MethodPost,
			path:   "/api/vmcp-instances",
		},
		{
			method: http.MethodGet,
			path:   "/api/all-mcps/servers",
		},
		{
			method: http.MethodGet,
			path:   "/v0.1/servers",
		},
		// An agent's key has no composite scope, which only Obot's own loopback tokens carry.
		{
			method: http.MethodPost,
			path:   "/mcp-connect-composite/" + agentTestVMCP,
		},
		// Devices.
		{
			method: http.MethodPost,
			path:   "/api/devices/scans",
		},
		{
			method: http.MethodGet,
			path:   "/api/devices/scans",
		},
		{
			method: http.MethodGet,
			path:   "/api/devices/scans/1",
		},
		{
			method: http.MethodPost,
			path:   "/api/local-agent-audit-logs",
		},
		{
			method: http.MethodPost,
			path:   "/api/enforcement/decisions",
		},
		{
			method: http.MethodPost,
			path:   "/api/mdm/enroll",
		},
		// Interactive sign-in and OAuth, where the handlers read api.Context.UserID, which is 0 for an agent.
		{
			method: http.MethodPost,
			path:   "/api/token-request",
		},
		{
			method: http.MethodGet,
			path:   "/api/token-request/tr1",
		},
		{
			method: http.MethodPost,
			path:   "/api/token-request/verify",
		},
		{
			method: http.MethodGet,
			path:   "/api/oauth/start/tr1/default/github",
		},
		{
			method: http.MethodGet,
			path:   "/api/oauth/redirect/default/github",
		},
		{
			method: http.MethodGet,
			path:   "/api/setup/oauth-complete",
		},
		{
			method: http.MethodGet,
			path:   "/api/app-oauth/authorize/app1",
		},
		{
			method: http.MethodGet,
			path:   "/oauth2/start",
		},
		{
			method: http.MethodGet,
			path:   "/oauth/authorize",
		},
		{
			method: http.MethodGet,
			path:   "/oauth/authorize/" + agentTestVMCP,
		},
		{
			method: http.MethodPost,
			path:   "/oauth/token",
		},
		{
			method: http.MethodPost,
			path:   "/oauth/register",
		},
		{
			method: http.MethodGet,
			path:   "/oauth/callback/oar1",
		},
		{
			method: http.MethodGet,
			path:   "/oauth/mcp/callback",
		},
		{
			method: http.MethodGet,
			path:   "/oauth/consent/oar1",
		},
		{
			method: http.MethodPost,
			path:   "/oauth/consent/oar1/approve",
		},
		{
			method: http.MethodGet,
			path:   "/oauth/mcp/login/state1",
		},
		{
			method: http.MethodGet,
			path:   "/oauth/userinfo",
		},
		{
			method: http.MethodGet,
			path:   "/api/oauth/vmcp/" + agentTestVMCP,
		},
		{
			method: http.MethodGet,
			path:   "/api/bootstrap",
		},
		{
			method: http.MethodPost,
			path:   "/api/bootstrap/login",
		},
		{
			method: http.MethodPost,
			path:   "/api/local-auth/activate",
		},
		// Routes open to any caller.
		{
			method: http.MethodGet,
			path:   "/api/healthz",
		},
		{
			method: http.MethodGet,
			path:   "/api/app-preferences",
		},
		{
			method: http.MethodGet,
			path:   "/.well-known/oauth-protected-resource/mcp-connect/" + agentTestVMCP,
		},
		{
			method: http.MethodGet,
			path:   "/api/image/img1",
		},
		// Skills other than reading them.
		{
			method: http.MethodPost,
			path:   "/api/skills",
		},
		{
			method: http.MethodGet,
			path:   "/api/skill-repositories",
		},
		// SCIM and the UI.
		{
			method: http.MethodGet,
			path:   "/scim/v2/Users",
		},
		{
			method: http.MethodGet,
			path:   "/",
			ui:     true,
		},
		{
			method: http.MethodGet,
			path:   "/admin/",
			ui:     true,
		},
		{
			method: http.MethodGet,
			path:   "/admin/users",
			ui:     true,
		},
	}

	// agentSkillRoutes are the skill GET routes an agent's key with the Skills scope reaches.
	agentSkillRoutes = []agentTestRoute{
		{
			method: http.MethodGet,
			path:   "/api/skills",
		},
		{
			method: http.MethodGet,
			path:   "/api/skills/" + agentTestSkill,
		},
		{
			method: http.MethodGet,
			path:   "/api/skills/" + agentTestSkill + "/download",
		},
		{
			method: http.MethodGet,
			path:   "/api/skills/" + agentTestSkill + "/preview",
		},
	}
)

type agentTestRoute struct {
	method string
	path   string
	// ui marks a route that the server's mux serves with the UI fallback.
	ui bool
}

func newAgentTestAuthorizer(t *testing.T) *Authorizer {
	t.Helper()

	// A shared vMCP whose profile grants every subject, which grants the agent too.
	vmcp := &v1.VMCP{
		Name:      agentTestVMCP,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: []types.VMCPComponent{
					{
						ID: "component",
					},
				},
				Profiles: []types.VMCPProfile{
					{
						Subjects: []types.Subject{
							{
								Type: types.SubjectTypeSelector,
								ID:   "*",
							},
						},
					},
				},
			},
		},
	}
	skill := &v1.Skill{
		Name:      agentTestSkill,
		Namespace: system.DefaultNamespace,
		Spec: v1.SkillSpec{
			RepoID: "repo-1",
		},
		Status: v1.SkillStatus{
			Valid: true,
		},
	}
	systemServer := &v1.SystemMCPServer{
		Name:      agentTestSystemServer,
		Namespace: system.DefaultNamespace,
	}
	storage := newMCPIDIsAuthorizedTestStorage(vmcp, skill, systemServer)

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, skillaccessrule.Indexers())
	if err := indexer.Add(&v1.SkillAccessRule{
		Name:      "allow-all",
		Namespace: system.DefaultNamespace,
		Spec: v1.SkillAccessRuleSpec{
			Manifest: types.SkillAccessRuleManifest{
				Subjects: []types.Subject{
					{
						Type: types.SubjectTypeSelector,
						ID:   "*",
					},
				},
				Resources: []types.SkillResource{
					{
						Type: types.SkillResourceTypeSelector,
						ID:   "*",
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("add skill access rule: %v", err)
	}

	return NewAuthorizer(nil, storage, storage, false, nil, skillaccessrule.NewHelper(indexer), nil)
}

// agentKeyPrincipal returns the principal of agent 7 as the API key authenticator builds it for a key with scopes.
func agentKeyPrincipal(scopes gatewaytypes.APIKeyScopes) user.Info {
	return gatewayclient.AgentUserInfo(&gatewaytypes.Agent{
		ID:          7,
		DisplayName: "nightly build",
	}, scopes.Groups(nil), map[string][]string{
		"authorized_mcp_ids": scopes.MCPServerIDs,
	})
}

func agentTestRequest(route agentTestRoute) *http.Request {
	req := httptest.NewRequest(route.method, route.path, nil)
	if route.ui {
		// The server's mux serves the UI as its "/" fallback, and sets the pattern it matched.
		req.Pattern = "/"
	}
	return req
}

// TestAgentRouteReachability checks an agent's principal against the route rules: with every scope an agent's key
// can have, it reaches only the LLM proxy, the skill GET routes, MCP connections and the API key webhook.
func TestAgentRouteReachability(t *testing.T) {
	authorizer := newAgentTestAuthorizer(t)
	agent := agentKeyPrincipal(gatewaytypes.APIKeyScopes{
		CanAccessSkills:   true,
		CanAccessLLMProxy: true,
		MCPServerIDs:      []string{"*"},
	})

	for _, route := range agentReachableRoutes {
		t.Run("reaches "+route.method+" "+route.path, func(t *testing.T) {
			if !authorizer.Authorize(agentTestRequest(route), agent) {
				t.Fatal("the agent was denied")
			}
		})
	}
	for _, route := range agentUnreachableRoutes {
		t.Run("denied "+route.method+" "+route.path, func(t *testing.T) {
			if authorizer.Authorize(agentTestRequest(route), agent) {
				t.Fatal("the agent was authorized")
			}
		})
	}
}

// TestAgentRoutesFollowKeyScopes checks that an agent reaches each route only with the scope that grants it, and needs
// no other scope for it. In particular, a key without an MCP scope connects to no vMCP, not even one the agent is
// granted: an empty authorized_mcp_ids restricts nothing, so the key is kept out only because it has no GroupMCP. And a
// key without the Skills scope reaches no skill route, although a skill access rule grants every skill to everyone.
func TestAgentRoutesFollowKeyScopes(t *testing.T) {
	authorizer := newAgentTestAuthorizer(t)
	llmRoutes := slices.DeleteFunc(slices.Clone(agentReachableRoutes), func(route agentTestRoute) bool {
		return !strings.HasPrefix(route.path, "/api/llm-proxy/")
	})
	connect := agentTestRoute{
		method: http.MethodPost,
		path:   "/mcp-connect/" + agentTestVMCP,
	}

	for _, tt := range []struct {
		name        string
		scopes      gatewaytypes.APIKeyScopes
		wantLLM     bool
		wantSkills  bool
		wantConnect bool
	}{
		{
			name: "LLM proxy only",
			scopes: gatewaytypes.APIKeyScopes{
				CanAccessLLMProxy: true,
			},
			wantLLM: true,
		},
		{
			name: "Skills only",
			scopes: gatewaytypes.APIKeyScopes{
				CanAccessSkills: true,
			},
			wantSkills: true,
		},
		{
			name: "LLM proxy and Skills",
			scopes: gatewaytypes.APIKeyScopes{
				CanAccessLLMProxy: true,
				CanAccessSkills:   true,
			},
			wantLLM:    true,
			wantSkills: true,
		},
		{
			name: "the granted vMCP",
			scopes: gatewaytypes.APIKeyScopes{
				MCPServerIDs: []string{agentTestVMCP},
			},
			wantConnect: true,
		},
		{
			name: "another vMCP",
			scopes: gatewaytypes.APIKeyScopes{
				MCPServerIDs: []string{"vmcp1other"},
			},
		},
		{
			name: "LLM proxy and every vMCP",
			scopes: gatewaytypes.APIKeyScopes{
				CanAccessLLMProxy: true,
				MCPServerIDs:      []string{"*"},
			},
			wantLLM:     true,
			wantConnect: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			agent := agentKeyPrincipal(tt.scopes)
			check := func(routes []agentTestRoute, want bool) {
				t.Helper()
				for _, route := range routes {
					if got := authorizer.Authorize(agentTestRequest(route), agent); got != want {
						t.Errorf("%s %s: authorized = %v, want %v", route.method, route.path, got, want)
					}
				}
			}
			check(llmRoutes, tt.wantLLM)
			check(agentSkillRoutes, tt.wantSkills)
			check([]agentTestRoute{connect}, tt.wantConnect)
		})
	}
}

// TestAgentWithoutMCPScopeIDsReachesNoMCPConnection checks that an agent principal with GroupMCP but no
// authorized_mcp_ids connects to nothing, rather than to every vMCP it is granted.
func TestAgentWithoutMCPScopeIDsReachesNoMCPConnection(t *testing.T) {
	authorizer := newAgentTestAuthorizer(t)
	for _, groups := range [][]string{
		{types.GroupMCP, types.GroupAuthenticated},
		{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated},
	} {
		agent := gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, groups, nil)
		for _, path := range []string{"/mcp-connect/" + agentTestVMCP, "/mcp-connect-composite/" + agentTestVMCP} {
			if authorizer.Authorize(httptest.NewRequest(http.MethodPost, path, nil), agent) {
				t.Errorf("groups %v: an agent with no authorized MCP IDs reached %s", groups, path)
			}
		}
	}
}

// TestAgentCompositeLoopbackReachesItsVMCP checks that the principal of a composite loopback token issued to an agent
// reaches the composite connection it names, as a user's does.
func TestAgentCompositeLoopbackReachesItsVMCP(t *testing.T) {
	authorizer := newAgentTestAuthorizer(t)
	agent := gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated}, map[string][]string{
		"authorized_mcp_ids": {"component", agentTestVMCP},
	})
	if !authorizer.Authorize(httptest.NewRequest(http.MethodPost, "/mcp-connect-composite/"+agentTestVMCP, nil), agent) {
		t.Fatal("the agent's composite loopback was denied")
	}
	if authorizer.Authorize(httptest.NewRequest(http.MethodGet, "/api/me", nil), agent) {
		t.Fatal("the agent's composite loopback reached a user route")
	}
}

// TestAgentPrincipalsAreConfinedWhateverTheirGroups checks that a principal that claims to be an agent is never
// authorized as a user: not with a group that no agent is granted, and not when its UID and recorded kind disagree.
func TestAgentPrincipalsAreConfinedWhateverTheirGroups(t *testing.T) {
	authorizer := newAgentTestAuthorizer(t)
	allScopes := []string{types.GroupLLM, types.GroupSkills, types.GroupMCP, types.GroupAuthenticated}
	agentExtra := func(kind principal.Kind) map[string][]string {
		extra := map[string][]string{
			"authorized_mcp_ids": {"*"},
		}
		if kind != "" {
			principal.RecordKind(extra, kind)
		}
		principal.RecordUserStatus(extra, types.UserStatusActive)
		return extra
	}

	for _, tt := range []struct {
		name string
		user user.Info
	}{
		{
			name: "agent with GroupAPI",
			user: gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, append(allScopes, types.GroupAPI), map[string][]string{
				"authorized_mcp_ids": {"*"},
			}),
		},
		{
			name: "agent with an administrator's groups",
			user: gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, append(allScopes, types.RoleAdmin.Groups()...), map[string][]string{
				"authorized_mcp_ids": {"*"},
			}),
		},
		{
			name: "agent with device scans",
			user: gatewayclient.AgentUserInfo(&gatewaytypes.Agent{ID: 7}, append(allScopes, types.GroupDeviceScans), map[string][]string{
				"authorized_mcp_ids": {"*"},
			}),
		},
		{
			name: "agent UID recorded as a user",
			user: &user.DefaultInfo{
				Name:   "agent:7",
				UID:    "agent:7",
				Groups: append(allScopes, types.RoleAdmin.Groups()...),
				Extra:  agentExtra(principal.KindUser),
			},
		},
		{
			name: "user UID recorded as an agent",
			user: &user.DefaultInfo{
				Name:   "7",
				UID:    "7",
				Groups: append(allScopes, types.RoleAdmin.Groups()...),
				Extra:  agentExtra(principal.KindAgent),
			},
		},
		{
			name: "agent UID with no recorded kind",
			user: &user.DefaultInfo{
				Name:   "agent:7",
				UID:    "agent:7",
				Groups: append(allScopes, types.RoleAdmin.Groups()...),
				Extra:  agentExtra(""),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, route := range append(agentReachableRoutes, agentUnreachableRoutes...) {
				if authorizer.Authorize(agentTestRequest(route), tt.user) {
					t.Errorf("%s %s: authorized", route.method, route.path)
				}
			}
		})
	}
}

// TestUserWithTheAgentsIDIsNotConfined checks that the agent rules apply to the agent alone: the user with the
// agent's numeric ID keeps the routes of a user, and the agent gets none of them.
func TestUserWithTheAgentsIDIsNotConfined(t *testing.T) {
	authorizer := newAgentTestAuthorizer(t)
	extra := map[string][]string{
		"obot_groups": types.RoleBasic.RoleGroups(),
	}
	principal.RecordKind(extra, principal.KindUser)
	principal.RecordUserStatus(extra, types.UserStatusActive)
	user := &user.DefaultInfo{
		Name:   "alice",
		UID:    "7",
		Groups: types.RoleBasic.Groups(),
		Extra:  extra,
	}
	agent := agentKeyPrincipal(gatewaytypes.APIKeyScopes{
		CanAccessSkills:   true,
		CanAccessLLMProxy: true,
		MCPServerIDs:      []string{"*"},
	})

	for _, route := range []agentTestRoute{
		{
			method: http.MethodGet,
			path:   "/api/me",
		},
		{
			method: http.MethodPost,
			path:   "/api/api-keys",
		},
		{
			method: http.MethodGet,
			path:   "/api/users/7",
		},
		{
			method: http.MethodPost,
			path:   "/mcp-connect/" + agentTestVMCP,
		},
	} {
		if !authorizer.Authorize(agentTestRequest(route), user) {
			t.Errorf("%s %s: the user was denied", route.method, route.path)
		}
	}
	for _, route := range []agentTestRoute{
		{
			method: http.MethodGet,
			path:   "/api/me",
		},
		{
			method: http.MethodPost,
			path:   "/api/api-keys",
		},
		{
			method: http.MethodGet,
			path:   "/api/users/7",
		},
	} {
		if authorizer.Authorize(agentTestRequest(route), agent) {
			t.Errorf("%s %s: the agent with the user's ID was authorized", route.method, route.path)
		}
	}
}

// TestAgentsDoNotConnectToSystemMCPServers checks that an agent connects only through vMCPs: a system MCP server, which
// a user whose key allows every MCP server reaches, is not reachable by an agent whose key does.
func TestAgentsDoNotConnectToSystemMCPServers(t *testing.T) {
	authorizer := newAgentTestAuthorizer(t)
	scopes := gatewaytypes.APIKeyScopes{
		MCPServerIDs: []string{"*"},
	}
	extra := map[string][]string{
		"authorized_mcp_ids": scopes.MCPServerIDs,
	}
	principal.RecordKind(extra, principal.KindUser)
	principal.RecordUserStatus(extra, types.UserStatusActive)
	user := &user.DefaultInfo{
		Name:   "alice",
		UID:    "7",
		Groups: scopes.Groups(nil),
		Extra:  extra,
	}
	agent := agentKeyPrincipal(scopes)

	req := func() *http.Request {
		return httptest.NewRequest(http.MethodPost, "/mcp-connect/"+agentTestSystemServer, nil)
	}
	if !authorizer.Authorize(req(), user) {
		t.Fatal("the user's key that allows every MCP server was denied the system server")
	}
	if authorizer.Authorize(req(), agent) {
		t.Fatal("the agent reached a system MCP server")
	}
}

// TestSkillAdminBypassNeverAppliesToAgents checks that the administrator and auditor bypass of skill access rules
// never applies to an agent, even one marked as either: only the rules that match the agent grant it a skill. The
// bypass still applies to users.
func TestSkillAdminBypassNeverAppliesToAgents(t *testing.T) {
	skill := &v1.Skill{
		Name:      agentTestSkill,
		Namespace: system.DefaultNamespace,
		Spec: v1.SkillSpec{
			RepoID: "repo-1",
		},
		Status: v1.SkillStatus{
			Valid: true,
		},
	}
	storage := newMCPIDIsAuthorizedTestStorage(skill)
	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, skillaccessrule.Indexers())
	// The skill is granted to user 7 only, never to agent 7.
	if err := indexer.Add(&v1.SkillAccessRule{
		Name:      "user-7",
		Namespace: system.DefaultNamespace,
		Spec: v1.SkillAccessRuleSpec{
			Manifest: types.SkillAccessRuleManifest{
				Subjects: []types.Subject{
					{
						Type: types.SubjectTypeUser,
						ID:   "7",
					},
				},
				Resources: []types.SkillResource{
					{
						Type: types.SkillResourceTypeSkill,
						ID:   agentTestSkill,
					},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	authorizer := NewAuthorizer(nil, storage, storage, false, nil, skillaccessrule.NewHelper(indexer), nil)

	agent := agentKeyPrincipal(gatewaytypes.APIKeyScopes{
		CanAccessSkills: true,
	})
	userExtra := map[string][]string{}
	principal.RecordKind(userExtra, principal.KindUser)
	otherUser := &user.DefaultInfo{
		Name:  "admin",
		UID:   "9",
		Extra: userExtra,
	}

	for _, tt := range []struct {
		name string
		user User
		want bool
	}{
		{
			name: "agent",
			user: User{
				Info: agent,
			},
			want: false,
		},
		{
			name: "agent marked as an administrator",
			user: User{
				Info:           agent,
				IsAdmin:        true,
				CanReadAsAdmin: true,
			},
			want: false,
		},
		{
			name: "agent marked as an auditor",
			user: User{
				Info:      agent,
				IsAuditor: true,
			},
			want: false,
		},
		{
			name: "agent marked as a read-only administrator",
			user: User{
				Info:           agent,
				CanReadAsAdmin: true,
			},
			want: false,
		},
		{
			name: "agent marked as the owner and an administrator",
			user: User{
				Info:           agent,
				IsAdmin:        true,
				CanReadAsAdmin: true,
				IsOwner:        true,
			},
			want: false,
		},
		{
			name: "administrator no rule grants the skill",
			user: User{
				Info:           otherUser,
				IsAdmin:        true,
				CanReadAsAdmin: true,
			},
			want: true,
		},
		{
			name: "read-only administrator no rule grants the skill",
			user: User{
				Info:           otherUser,
				CanReadAsAdmin: true,
			},
			want: true,
		},
		{
			name: "auditor no rule grants the skill",
			user: User{
				Info:      otherUser,
				IsAuditor: true,
			},
			want: true,
		},
		{
			name: "user no rule grants the skill",
			user: User{
				Info: otherUser,
			},
			want: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/skills/"+agentTestSkill, nil)
			got, err := authorizer.checkSkill(req, &Resources{SkillID: agentTestSkill}, tt.user)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("checkSkill = %v, want %v", got, tt.want)
			}
		})
	}

	// Through the authorizer, an agent's principal is never marked as an administrator, read-only administrator or
	// auditor, and the rule for user 7 does not grant agent 7 the skill.
	for _, route := range agentSkillRoutes[1:] {
		if authorizer.Authorize(agentTestRequest(route), agent) {
			t.Errorf("%s %s: the agent reached a skill no rule grants it", route.method, route.path)
		}
	}
}
