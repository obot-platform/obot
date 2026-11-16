package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	// HeaderObotAPIKey carries an Obot API key alongside another credential, on LLM proxy
	// requests.
	HeaderObotAPIKey = auth.HeaderObotAPIKey

	llmProxyPathPrefix = "/api/llm-proxy/"
)

// APIKeyAuthenticator authenticates requests using the API keys of users (ok1-) and of agents (oak1-). A key's
// principal has only the groups its scopes grant, not every group of its owner.
//
// The server adds it to the authenticator chain only when authentication is enabled. Agents authenticate only with
// their keys, so without authentication they are not supported: an agent key is then tried by the rest of the chain
// as any other bearer value is, and authenticates no one.
type APIKeyAuthenticator struct {
	client *client.Client
}

// NewAPIKeyAuthenticator creates a new API key authenticator.
func NewAPIKeyAuthenticator(client *client.Client) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{client: client}
}

// AuthenticateRequest implements authenticator.Request.
func (a *APIKeyAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	// X-Obot-Api-Key lets a client whose Authorization header carries another credential,
	// such as Claude Code signed in with a Claude subscription, send its Obot API key too. It
	// is only for the LLM proxy, which strips it before forwarding; elsewhere, such as on MCP
	// connections, it would travel on to an upstream server.
	var authHeader string
	if strings.HasPrefix(req.URL.Path, llmProxyPathPrefix) {
		authHeader = req.Header.Get(HeaderObotAPIKey)
	}
	if authHeader == "" {
		authHeader = strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	}
	if authHeader == "" {
		authHeader = req.Header.Get("X-API-Key")
		if authHeader == "" {
			return nil, false, nil
		}
	}

	// The prefix names the kind of the key's owner, and ValidateAPIKey looks the key up as a key of that kind of
	// owner only, so the key it returns is one of that kind of owner: a key is never authenticated as an owner of
	// another kind, such as the user with its agent's numeric ID.
	ownerKind, ownerID, _, _, err := client.ParseAPIKey(authHeader)
	if err != nil {
		// Not an API key, or not a well-formed one, which ValidateAPIKey would refuse: let other authenticators try.
		return nil, false, nil
	}

	// Validate the API key
	apiKey, err := a.client.ValidateAPIKey(req.Context(), authHeader)
	if errors.Is(err, client.ErrInvalidAPIKey) {
		// Return false, nil to let other authenticators try
		// This allows the chain to continue if the key is invalid
		return nil, false, nil
	} else if err != nil {
		// The key or its owner could not be read, so its owner's access is unknown. Fail rather than letting the
		// request continue to another credential or to anonymous access.
		if ownerKind == principal.KindAgent {
			return nil, false, &client.AgentAccessLookupError{
				AgentID: ownerID,
				Err:     err,
			}
		}
		return nil, false, &client.UserAccessLookupError{
			UserID: ownerID,
			Err:    err,
		}
	}

	if ownerKind == principal.KindAgent {
		return a.authenticateAgentKey(req, apiKey)
	}
	return a.authenticateUserKey(req, apiKey)
}

// authenticateUserKey returns the principal of the user who owns apiKey.
func (a *APIKeyAuthenticator) authenticateUserKey(req *http.Request, apiKey *gatewaytypes.APIKey) (*authenticator.Response, bool, error) {
	userID, ok := apiKey.Owner().UserID()
	if !ok {
		return nil, false, nil
	}

	// Get the user from the database
	u, authProviderGroups, err := a.client.UserByIDWithEffectiveRole(req.Context(), userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// The key outlived its user. Deny it rather than letting the request continue as anonymous.
		return nil, false, &client.UserAccessDeniedError{
			UserID: userID,
			Status: types2.UserStatusDeleted,
		}
	} else if err != nil {
		return nil, false, &client.UserAccessLookupError{
			UserID: userID,
			Err:    err,
		}
	}

	attribution := principal.NewAPIKeyAttribution(apiKey.ID, principal.User(userID), apiKey.Name)
	groups := apiKey.Groups(u)
	if apiKey.ClaudeEnvironment {
		groups = append(groups, types2.GroupClaudeEnvironment)
	}
	extra := map[string][]string{
		"email":                      {u.Email},
		"authorized_mcp_ids":         apiKey.MCPServerIDs,
		"obot_groups":                u.Role.RoleGroups(),
		principal.APIKeyIDExtra:      {fmt.Sprintf("%d", attribution.ID)},
		principal.APIKeyNameExtra:    {attribution.Name},
		principal.APIKeyRawNameExtra: {apiKey.Name},
	}

	if authProviderGroups != nil {
		extra["auth_provider_groups"] = authProviderGroups
	}
	principal.RecordPrincipal(extra, principal.KindUser, u.Status())

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   u.Username,
			UID:    fmt.Sprintf("%d", u.ID),
			Groups: groups,
			Extra:  extra,
		},
	}, true, nil
}

// authenticateAgentKey returns the principal of the agent that owns apiKey. It carries no user privileges: no role,
// no group of a user, and only the groups agentKeyGroups grants.
func (a *APIKeyAuthenticator) authenticateAgentKey(req *http.Request, apiKey *gatewaytypes.APIKey) (*authenticator.Response, bool, error) {
	agentID, ok := apiKey.Owner().AgentID()
	if !ok {
		return nil, false, nil
	}

	// ValidateAPIKey does not refuse the key of an owner that is not active. The agent is read on every request, so a
	// disabled or deleted agent is denied from its next request on, and it is denied here rather than left to the
	// admission check, so that no use of this authenticator can admit it.
	agent, err := a.client.ActiveAgent(req.Context(), agentID)
	if err != nil {
		return nil, false, err
	}

	attribution := principal.NewAPIKeyAttribution(apiKey.ID, principal.Agent(agentID), apiKey.Name)
	extra := map[string][]string{
		"authorized_mcp_ids":         apiKey.MCPServerIDs,
		principal.APIKeyIDExtra:      {fmt.Sprintf("%d", attribution.ID)},
		principal.APIKeyNameExtra:    {attribution.Name},
		principal.APIKeyRawNameExtra: {apiKey.Name},
	}
	// AgentUserInfo records the agent's kind and status, so that the admission check reads this principal as an
	// active agent.
	return &authenticator.Response{
		User: client.AgentUserInfo(agent, agentKeyGroups(apiKey.APIKeyScopes), extra),
	}, true, nil
}

// agentKeyGroups returns the groups that an agent's API key grants. They are what its scopes (agentKeyScopes) grant,
// as for a user's key, and never more. An agent has no role, so Groups is given no user and adds no role group.
// GroupMCP comes only with an MCP scope, so a key without one cannot reach MCP routes. Claude environment credentials
// are only issued to users, so no agent gets that group either.
func agentKeyGroups(scopes gatewaytypes.APIKeyScopes) []string {
	return agentKeyScopes(scopes).Groups(nil)
}

// agentKeyScopes returns the scopes an agent's API key is honored with: its stored scopes, without API or device scan
// access. An agent's key never grants either; both are cleared here even should a stored key carry them, so an agent
// never gets GroupAPI, which reaches every user route, API key management (POST /api/api-keys) among them, and the API
// key webhook never reports either for an agent's key.
func agentKeyScopes(scopes gatewaytypes.APIKeyScopes) gatewaytypes.APIKeyScopes {
	scopes.CanAccessAPI = false
	scopes.CanAccessDeviceScans = false
	return scopes
}
