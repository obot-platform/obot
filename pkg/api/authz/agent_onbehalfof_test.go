package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

const (
	// onBehalfOfTestVMCP is a shared vMCP with two profiles that name agent 7, onBehalfOfTestOtherVMCP another one with
	// a profile that names it, and onBehalfOfTestUsersVMCP one whose profile names user 9 and every user, but not the
	// agent.
	onBehalfOfTestVMCP      = "vmcp1obo"
	onBehalfOfTestOtherVMCP = "vmcp1obootherprofile"
	onBehalfOfTestUsersVMCP = "vmcp1obousers"
	onBehalfOfTestRead      = "vp1read"
	onBehalfOfTestAll       = "vp1all"
	onBehalfOfTestOther     = "vp1other"
	onBehalfOfTestUsers     = "vp1users"
)

var (
	// allAgentScopes are every scope an agent's key can have.
	allAgentScopes = gatewaytypes.APIKeyScopes{
		CanAccessSkills:   true,
		CanAccessLLMProxy: true,
		MCPServerIDs:      []string{"*"},
	}
)

// onBehalfOfPrincipal returns the principal of agent 7, with a key with scopes, acting on behalf of user 9, as the
// authenticator of delegated tokens builds it.
func onBehalfOfPrincipal(t *testing.T, scopes gatewaytypes.APIKeyScopes) user.Info {
	t.Helper()

	info := agentKeyPrincipal(scopes)
	principal.RecordOnBehalfOf(info.GetExtra(), 9)
	require.NoError(t, agentprofile.RecordDelegation(info.GetExtra(), agentprofile.Delegation{
		ID:   "adl1test",
		User: subjects.Caller{Principal: principal.User(9)},
	}))
	return info
}

// newOnBehalfOfTestAuthorizer returns an authorizer with the vMCPs of onBehalfOfTestVMCP, onBehalfOfTestOtherVMCP and
// onBehalfOfTestUsersVMCP, and a skill that the skill access rule "agent-rule", which names agent 7, grants.
func newOnBehalfOfTestAuthorizer(t *testing.T) *Authorizer {
	t.Helper()

	agent := []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}}
	vmcp := func(name string, profiles ...types.VMCPProfile) *v1.VMCP {
		return &v1.VMCP{
			Name:      name,
			Namespace: system.DefaultNamespace,
			Spec: v1.VMCPSpec{
				Manifest: types.VMCPManifest{
					Components: []types.VMCPComponent{{ID: "component"}},
					Profiles:   profiles,
				},
			},
		}
	}
	storage := newMCPIDIsAuthorizedTestStorage(
		vmcp(onBehalfOfTestVMCP,
			types.VMCPProfile{ID: onBehalfOfTestRead, Name: "read", Subjects: agent},
			types.VMCPProfile{ID: onBehalfOfTestAll, Name: "all", Subjects: agent},
		),
		vmcp(onBehalfOfTestOtherVMCP,
			types.VMCPProfile{ID: onBehalfOfTestOther, Name: "other", Subjects: agent},
		),
		vmcp(onBehalfOfTestUsersVMCP,
			types.VMCPProfile{ID: onBehalfOfTestUsers, Name: "users", Subjects: []types.Subject{
				{Type: types.SubjectTypeUser, ID: "9"},
				{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID},
			}},
		),
		&v1.Skill{
			Name:      agentTestSkill,
			Namespace: system.DefaultNamespace,
			Spec:      v1.SkillSpec{RepoID: "repo-1"},
			Status:    v1.SkillStatus{Valid: true},
		},
	)

	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, skillaccessrule.Indexers())
	require.NoError(t, indexer.Add(&v1.SkillAccessRule{
		Name:      "agent-rule",
		Namespace: system.DefaultNamespace,
		Spec: v1.SkillAccessRuleSpec{
			Manifest: types.SkillAccessRuleManifest{
				Subjects:  agent,
				Resources: []types.SkillResource{{Type: types.SkillResourceTypeSelector, ID: "*"}},
			},
		},
	}))

	return NewAuthorizer(nil, storage, storage, false, nil, skillaccessrule.NewHelper(indexer), nil)
}

func TestAcceptsDelegatedTokens(t *testing.T) {
	for _, tt := range []struct {
		method string
		path   string
		want   bool
	}{
		{method: http.MethodPost, path: "/api/llm-proxy/v1/messages", want: true},
		{method: http.MethodGet, path: "/api/llm-proxy/v1/models", want: true},
		{method: http.MethodPost, path: "/api/llm-proxy/openai/v1/chat/completions", want: true},
		{method: http.MethodGet, path: "/api/skills", want: true},
		{method: http.MethodGet, path: "/api/skills/sk1x", want: true},
		{method: http.MethodGet, path: "/api/skills/sk1x/download", want: true},
		{method: http.MethodGet, path: "/api/skills/sk1x/preview", want: true},
		{method: http.MethodPost, path: "/mcp-connect/vmcp1x", want: true},
		{method: http.MethodGet, path: "/mcp-connect/vmcp1x", want: true},
		{method: http.MethodDelete, path: "/mcp-connect/vmcp1x", want: true},
		{method: http.MethodPost, path: "/mcp-connect-composite/vmcpi1x", want: true},
		// The agent's other routes, which only its own key or an API key reaches.
		{method: http.MethodPost, path: "/api/api-keys/auth"},
		{method: http.MethodPost, path: "/api/agent-delegations"},
		{method: http.MethodPost, path: "/api/agent-delegations/token"},
		// And no route of anyone else.
		{method: http.MethodPost, path: "/api/skills"},
		{method: http.MethodDelete, path: "/api/skills/sk1x"},
		{method: http.MethodGet, path: "/api/me"},
		{method: http.MethodGet, path: "/api/me/agent-delegations"},
		{method: http.MethodGet, path: "/api/vmcps"},
		{method: http.MethodGet, path: "/api/mcp-audit-logs"},
		{method: http.MethodGet, path: "/"},
	} {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			require.Equal(t, tt.want, AcceptsDelegatedTokens(httptest.NewRequest(tt.method, tt.path, nil)))
		})
	}
}

// TestAgentActingOnBehalfOfAUserReachesOnlyDelegatedRoutes checks that an agent acting on behalf of a user, with every
// scope, reaches the LLM proxy, the skill GET routes and MCP connections, and
// none of the agent's other routes: not the API key webhook, and never the routes of the agent's own key, with which it
// could mint more tokens.
func TestAgentActingOnBehalfOfAUserReachesOnlyDelegatedRoutes(t *testing.T) {
	authorizer := newOnBehalfOfTestAuthorizer(t)
	caller := onBehalfOfPrincipal(t, allAgentScopes)

	for _, route := range []agentTestRoute{
		{method: http.MethodPost, path: "/api/llm-proxy/v1/messages"},
		{method: http.MethodGet, path: "/api/llm-proxy/v1/models"},
		{method: http.MethodGet, path: "/api/skills"},
		{method: http.MethodGet, path: "/api/skills/" + agentTestSkill},
		{method: http.MethodGet, path: "/api/skills/" + agentTestSkill + "/download"},
		{method: http.MethodGet, path: "/mcp-connect/" + onBehalfOfTestVMCP},
		{method: http.MethodPost, path: "/mcp-connect/" + onBehalfOfTestVMCP},
	} {
		t.Run("reaches "+route.method+" "+route.path, func(t *testing.T) {
			require.True(t, authorizer.Authorize(agentTestRequest(route), caller))
		})
	}

	denied := append([]agentTestRoute{
		{method: http.MethodPost, path: "/api/api-keys/auth"},
		{method: http.MethodPost, path: "/api/agent-delegations"},
		{method: http.MethodPost, path: "/api/agent-delegations/token"},
	}, agentUnreachableRoutes...)
	for _, route := range denied {
		t.Run("denied "+route.method+" "+route.path, func(t *testing.T) {
			// The request presents the agent's own key, so that only the on-behalf-of user keeps it from the routes of
			// that key.
			require.False(t, authorizer.Authorize(agentTestRequest(route), caller))
		})
	}
}

// TestOnBehalfOfUsesTheAgentsGrantsWithinTheKeysScopes checks that an agent acting on behalf of a user reaches what its
// own grants give it, and never what the user's give them, and that the scopes of the key that minted its token cap
// everything.
func TestOnBehalfOfUsesTheAgentsGrantsWithinTheKeysScopes(t *testing.T) {
	authorizer := newOnBehalfOfTestAuthorizer(t)
	llm := agentTestRoute{method: http.MethodPost, path: "/api/llm-proxy/v1/messages"}
	skillList := agentTestRoute{method: http.MethodGet, path: "/api/skills"}
	skill := agentTestRoute{method: http.MethodGet, path: "/api/skills/" + agentTestSkill}
	connect := agentTestRoute{method: http.MethodPost, path: "/mcp-connect/" + onBehalfOfTestVMCP}
	connectOther := agentTestRoute{method: http.MethodPost, path: "/mcp-connect/" + onBehalfOfTestOtherVMCP}
	connectUsers := agentTestRoute{method: http.MethodPost, path: "/mcp-connect/" + onBehalfOfTestUsersVMCP}

	for _, tt := range []struct {
		name   string
		scopes gatewaytypes.APIKeyScopes
		want   map[agentTestRoute]bool
	}{
		{
			// The vMCP whose profile names the user, and every user, is never the agent's.
			name:   "every scope",
			scopes: allAgentScopes,
			want:   map[agentTestRoute]bool{llm: true, skillList: true, skill: true, connect: true, connectOther: true},
		},
		{
			name:   "a key without the LLM scope",
			scopes: gatewaytypes.APIKeyScopes{CanAccessSkills: true, MCPServerIDs: []string{"*"}},
			want:   map[agentTestRoute]bool{skillList: true, skill: true, connect: true, connectOther: true},
		},
		{
			name:   "a key without the skills scope",
			scopes: gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true, MCPServerIDs: []string{"*"}},
			want:   map[agentTestRoute]bool{llm: true, connect: true, connectOther: true},
		},
		{
			name:   "a key scoped to one vMCP",
			scopes: gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true, MCPServerIDs: []string{onBehalfOfTestOtherVMCP}},
			want:   map[agentTestRoute]bool{llm: true, connectOther: true},
		},
		{
			name:   "a key scoped to the users' vMCP",
			scopes: gatewaytypes.APIKeyScopes{MCPServerIDs: []string{onBehalfOfTestUsersVMCP}},
			want:   map[agentTestRoute]bool{},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			caller := onBehalfOfPrincipal(t, tt.scopes)
			for _, route := range []agentTestRoute{llm, skillList, skill, connect, connectOther, connectUsers} {
				require.Equal(t, tt.want[route], authorizer.Authorize(agentTestRequest(route), caller), "%s %s", route.method, route.path)
			}
		})
	}
}

// TestOnBehalfOfWithoutTheDelegationRecordReachesNothing checks that an agent acting on behalf of a user without what
// the authenticator of delegated tokens records about the user reaches nothing, not even what its own grants give it:
// only that authenticator builds such a caller, and it always records it.
func TestOnBehalfOfWithoutTheDelegationRecordReachesNothing(t *testing.T) {
	authorizer := newOnBehalfOfTestAuthorizer(t)
	for _, record := range [][]string{nil, {"{"}, {`{"id":"adl1test","userID":"8"}`}} {
		caller := onBehalfOfPrincipal(t, allAgentScopes)
		if record == nil {
			delete(caller.GetExtra(), principal.DelegationExtra)
		} else {
			caller.GetExtra()[principal.DelegationExtra] = record
		}
		for _, route := range []agentTestRoute{
			{method: http.MethodPost, path: "/api/llm-proxy/v1/messages"},
			{method: http.MethodGet, path: "/api/skills"},
			{method: http.MethodGet, path: "/api/skills/" + agentTestSkill},
			{method: http.MethodPost, path: "/mcp-connect/" + onBehalfOfTestVMCP},
		} {
			require.False(t, authorizer.Authorize(agentTestRequest(route), caller), "%v: %s %s", record, route.method, route.path)
		}
	}
}

// TestOnBehalfOfCallersNeverHaveTheUsersRolesOrGroups checks that an agent acting on behalf of a user who is an
// administrator, in groups that rules name, reaches nothing the user's role or groups would give it.
func TestOnBehalfOfCallersNeverHaveTheUsersRolesOrGroups(t *testing.T) {
	authorizer := newOnBehalfOfTestAuthorizer(t)
	caller := agentKeyPrincipal(allAgentScopes)
	principal.RecordOnBehalfOf(caller.GetExtra(), 9)
	require.NoError(t, agentprofile.RecordDelegation(caller.GetExtra(), agentprofile.Delegation{
		ID: "adl1test",
		User: subjects.Caller{
			Principal: principal.User(9),
			Groups: subjects.Groups{
				AuthProvider: map[string]struct{}{"eng": {}},
				Obot:         map[string]struct{}{types.GroupAdmin: {}, types.GroupOwner: {}},
			},
		},
	}))
	require.NotContains(t, caller.GetGroups(), types.GroupAdmin)
	for _, route := range agentUnreachableRoutes {
		require.False(t, authorizer.Authorize(agentTestRequest(route), caller), "%s %s", route.method, route.path)
	}
}
