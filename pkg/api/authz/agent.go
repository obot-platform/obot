package authz

import (
	"net/http"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

var (
	// agentGroups are the only groups an agent's principal can have: those that the scopes of an agent's API key, or
	// a token issued to an agent for an internal MCP connection, grant. An agent has no role, so it never has the
	// group of one.
	agentGroups = []string{
		types.GroupAuthenticated,
		types.GroupMCP,
		types.GroupCompositeMCP,
		types.GroupLLM,
		types.GroupSkills,
	}

	// agentRules are the routes an agent's principal can reach without a resource check, by the group that grants
	// each. An agent is authorized against these and agentAPIResources alone, never against the rules of users: it
	// reaches none of the routes open to any caller (anyGroup in staticRules), to any authenticated caller, or to the
	// UI. Many of those handlers assume a user, often through api.Context.UserID, which is 0 for an agent, so that an
	// agent would be read as user 0 there.
	agentRules = rulesFromStatic(map[string][]string{
		types.GroupLLM:    {
			"/api/llm-proxy/",
		},
		types.GroupSkills: {
			"GET /api/skills",
		},
		// The API key webhook answers for the key in the request's Authorization header, which the API key
		// authenticator authenticates the request with too. The handler answers for the key, whoever it is.
		anyGroup: {
			"POST /api/api-keys/auth",
		},
	})

	// agentAPIResources are the routes an agent's principal can reach with a resource check, by the group that grants
	// each. Their resources are checked as they are for a user.
	agentAPIResources = map[string]*pathMatcher{
		types.GroupMCP:          newPathMatcher(apiResources[types.GroupMCP]...),
		types.GroupCompositeMCP: newPathMatcher(apiResources[types.GroupCompositeMCP]...),
		types.GroupSkills:       newPathMatcher(apiResources[types.GroupSkills]...),
	}
)

// authorizeAgent authorizes a request of a principal that claims to be an agent (principal.ClaimsAgent). It is the
// only authorization such a principal gets: an agent reaches the LLM proxy with GroupLLM, the skill GET routes with
// GroupSkills, MCP connections with GroupMCP (and internal composite connections with GroupCompositeMCP), and the
// API key webhook, and nothing else.
func (a *Authorizer) authorizeAgent(req *http.Request, userInfo user.Info) bool {
	// A principal that claims to be an agent but is not a well-formed one, such as one whose UID and recorded kind
	// disagree, reaches nothing. Nor does one with a group that no agent is granted, such as GroupAPI or the group of a
	// role: no authenticator of agents grants one, so such a principal was not built by one.
	if !principal.IsAgent(userInfo) {
		return false
	}
	groups := userInfo.GetGroups()
	if slices.ContainsFunc(groups, func(group string) bool { return !slices.Contains(agentGroups, group) }) {
		return false
	}

	for _, r := range agentRules {
		if r.group == anyGroup || slices.Contains(groups, r.group) {
			if _, pattern := r.mux.Handler(req); pattern != "" {
				return true
			}
		}
	}

	// An agent never has a role, so it is never authorized as an administrator, auditor or owner.
	agent := User{Info: userInfo}
	for _, group := range groups {
		vars, ok := agentAPIResources[group].Match(req)
		if !ok {
			continue
		}
		// An empty authorized_mcp_ids restricts nothing (see userCanConnectToResolvedMCP). An agent's credential
		// always names what it may connect to: an API key without an MCP scope gets no GroupMCP, and a token issued
		// for an MCP connection names its server. So an agent with none reaches no MCP connection, rather than every
		// one it is granted.
		if (group == types.GroupMCP || group == types.GroupCompositeMCP) && len(userInfo.GetExtra()["authorized_mcp_ids"]) == 0 {
			continue
		}
		// Agents connect only through vMCPs. A system MCP server is authorized for any caller with a matching scope,
		// such as "*", and left to authorize its callers itself, so an agent is kept from it here.
		if system.IsSystemMCPServerID(vars("mcp_id")) {
			continue
		}
		if ok, err := a.evaluateResources(req, vars, agent); err == nil && ok {
			return true
		}
	}
	return false
}
