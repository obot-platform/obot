package server

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/hostedagentmodels"
	"github.com/obot-platform/obot/pkg/hostedagentrefs"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const apiKeyAuthPrefix = "ok1-"

// APIKeyAuthenticator authenticates requests using API keys.
// API key users have restricted access - they only get GroupAPIKey,
// not the full authenticated user groups.
type APIKeyAuthenticator struct {
	client *client.Client
	// storage resolves hosted agent instances for agent-bound keys. Those keys
	// carry no permissions of their own, so the instance has to be read on each
	// request.
	storage kclient.Client
}

// NewAPIKeyAuthenticator creates a new API key authenticator.
func NewAPIKeyAuthenticator(client *client.Client, storage kclient.Client) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{client: client, storage: storage}
}

// hostedAgentGroups is the complete group set a sandbox principal carries.
//
// These say only what the sandbox may reach. What it *is* is recorded by the
// owner extra on the principal, which principal.IsHostedAgent reads -- a group
// asserting "this is an agent" would duplicate a fact already carried there.
//
// GroupAPI is deliberately absent and must stay absent: it is the only thing
// keeping an exfiltrated agent credential away from the Obot API, since
// authorization elsewhere decides which servers and models an agent may use but
// nothing else restricts which endpoints accept it.
//
// GroupSkills is absent for the same reason. A sandbox never calls the skills
// API -- its skills are written to /etc/obot/skills as files, and the config
// gives each one a path rather than a URL -- while holding the group would let
// the credential list, preview and download every skill the installation has,
// since nothing narrows a skill request to the ones an agent was configured
// with.
func hostedAgentGroups(hasMCPServers bool) []string {
	groups := []string{
		types2.GroupAuthenticated,
		types2.GroupLLM,
	}
	if hasMCPServers {
		groups = append(groups, types2.GroupMCP)
	}
	return groups
}

// authenticateHostedAgent builds a principal for a sandbox.
//
// The agent's authorized MCP servers are read from the instance every time
// rather than from the key, which is what lets an administrator withdraw access
// and have it take effect at once — even while the sandbox is still running
// with stale configuration, and even if the restart that would replace it
// cannot be scheduled.
//
// The principal never carries GroupAPI, so an exfiltrated agent credential
// cannot reach the Obot API.
func (a *APIKeyAuthenticator) authenticateHostedAgent(req *http.Request, instanceID string, attribution principal.APIKeyAttribution) (*authenticator.Response, bool, error) {
	if a.storage == nil {
		return nil, false, nil
	}

	var instance v1.HostedAgentInstance
	if err := a.storage.Get(req.Context(), kclient.ObjectKey{
		Name:      instanceID,
		Namespace: system.DefaultNamespace,
	}, &instance); err != nil {
		// A key outliving its instance authenticates as nothing.
		return nil, false, nil
	}
	if !instance.DeletionTimestamp.IsZero() {
		return nil, false, nil
	}

	var agent v1.HostedAgent
	if err := a.storage.Get(req.Context(), kclient.ObjectKey{
		Name:      instance.Spec.HostedAgentName,
		Namespace: system.DefaultNamespace,
	}, &agent); err != nil {
		return nil, false, nil
	}

	// Template resources are granted by the administrator who published the
	// agent; instance resources were checked against the owner when they were
	// attached. Neither is re-checked against the owner here, by design.
	// Resolved through the same resolver the controller uses when it writes the
	// sandbox's /mcp-connect URLs. A template names servers portably, as
	// <source>::<key>, so leaving them unresolved here would have authorization
	// compare a local request ID against a reference that can never equal it --
	// denying an agent the very servers it was configured with.
	mcpIDs, unresolved := hostedagentrefs.New(a.storage, system.DefaultNamespace).
		MCPServers(req.Context(), slices.Concat(agent.Spec.Manifest.MCPServers, instance.Spec.Manifest.MCPServers))
	_ = unresolved // a reference naming nothing installed here grants nothing
	// Resolved by the same function the controller uses to write the sandbox's
	// model endpoints, so the credential grants exactly the models the config
	// offers. A second implementation could only ever disagree, and disagreeing
	// upward is a grant the agent was never configured to have: "*" would stay
	// an unrestricted wildcard at the proxy, and a model from a provider the
	// template excludes would still be callable, even though neither appears in
	// the configuration the agent is given.
	//
	// Provider restrictions come from the template alone, matching the
	// controller: an instance chooses among what its template allows, not
	// beyond it.
	models, err := hostedagentmodels.Resolve(req.Context(), a.storage, system.DefaultNamespace,
		slices.Concat(agent.Spec.Manifest.Models, instance.Spec.Manifest.Models),
		agent.Spec.Manifest.ModelProviders)
	if err != nil {
		return nil, false, fmt.Errorf("resolve hosted agent models: %w", err)
	}
	modelIDs := make([]string, 0, len(models))
	for _, model := range models {
		modelIDs = append(modelIDs, model.ID)
	}

	groups := hostedAgentGroups(len(mcpIDs) > 0)

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   "hosted-agent:" + instance.Name,
			UID:    "hosted-agent:" + instance.Name,
			Groups: groups,
			Extra: map[string][]string{
				// The same key a user API key populates, so downstream
				// authorization needs no agent-specific branch.
				"authorized_mcp_ids":              mcpIDs,
				principal.AuthorizedModelIDsExtra: modelIDs,
				"hosted_agent_instance_id":        {instance.Name},
				principal.HostedAgentOwnerExtra:   {instance.Spec.UserID},
				principal.APIKeyIDExtra:           {fmt.Sprintf("%d", attribution.ID)},
				principal.APIKeyNameExtra:         {attribution.Name},
			},
		},
	}, true, nil
}

// AuthenticateRequest implements authenticator.Request.
func (a *APIKeyAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	authHeader := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if authHeader == "" {
		authHeader = req.Header.Get("X-API-Key")
		if authHeader == "" {
			return nil, false, nil
		}
	}

	// Check if this is an API key (starts with ok1-)
	if !strings.HasPrefix(authHeader, apiKeyAuthPrefix) {
		return nil, false, nil
	}

	// Validate the API key
	apiKey, err := a.client.ValidateAPIKey(req.Context(), authHeader)
	if err != nil {
		// Return false, nil to let other authenticators try
		// This allows the chain to continue if the key is invalid
		return nil, false, nil
	}

	// A key bound to a hosted agent authenticates as that agent, never as the
	// user who created it, so it must not pick up the owner's identity, groups
	// or role below.
	if apiKey.HostedAgentInstanceID != nil && *apiKey.HostedAgentInstanceID != "" {
		return a.authenticateHostedAgent(req, *apiKey.HostedAgentInstanceID,
			principal.NewAPIKeyAttribution(apiKey.ID, apiKey.UserID, apiKey.Name))
	}

	// Get the user from the database
	u, err := a.client.UserByID(req.Context(), fmt.Sprintf("%d", apiKey.UserID))
	if err != nil {
		return nil, false, nil
	}

	attribution := principal.NewAPIKeyAttribution(apiKey.ID, apiKey.UserID, apiKey.Name)
	extra := map[string][]string{
		"email":                   {u.Email},
		"authorized_mcp_ids":      apiKey.MCPServerIDs,
		principal.APIKeyIDExtra:   {fmt.Sprintf("%d", attribution.ID)},
		principal.APIKeyNameExtra: {attribution.Name},
	}

	// Look up auth provider group memberships so that group-based access
	// rules (e.g. skill access policies) work for API-key-authenticated
	// requests such as those made by nanobot.
	if authGroupIDs, err := a.client.ListGroupIDsForUser(req.Context(), u.ID); err == nil {
		extra["auth_provider_groups"] = authGroupIDs
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   u.Username,
			UID:    fmt.Sprintf("%d", u.ID),
			Groups: apiKey.Groups(u),
			Extra:  extra,
		},
	}, true, nil
}
