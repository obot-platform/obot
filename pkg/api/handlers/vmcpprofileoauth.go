package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ProfileOAuthAuthorizer starts an administrator's authorization of a vMCP profile's OAuth grant. The MCP OAuth handler
// factory implements it; this interface breaks an import cycle.
type ProfileOAuthAuthorizer interface {
	StartProfileOAuthGrant(req api.Context, vmcpID, profileID, componentID string) (string, error)
}

// VMCPProfileOAuthHandler serves the routes that authorize and reset the OAuth grants a vMCP profile holds for its
// components, which every agent that resolves to the profile uses headlessly. They exist only for agents, so they
// answer 404 while agent identities are disabled, and 501 while authentication is, as the agent admin API does.
//
// The routes are admin-only, which the authorizer enforces: they are under the administrators' /api/vmcps/ rule, no
// other role or group is granted them, and they are not on the agents' allowlist. They return the grant's metadata
// (who authorized it, when, for which target and scopes), never a token.
type VMCPProfileOAuthHandler struct {
	enabled     bool
	authEnabled bool
	authorizer  ProfileOAuthAuthorizer
	grants      *vmcpconfig.ProfileOAuthGrantStore
}

func NewVMCPProfileOAuthHandler(enabled, authEnabled bool, authorizer ProfileOAuthAuthorizer, grants *vmcpconfig.ProfileOAuthGrantStore) *VMCPProfileOAuthHandler {
	return &VMCPProfileOAuthHandler{
		enabled:     enabled,
		authEnabled: authEnabled,
		authorizer:  authorizer,
		grants:      grants,
	}
}

// RegisterRoutes registers the routes with handle.
func (h *VMCPProfileOAuthHandler) RegisterRoutes(handle func(pattern string, f api.HandlerFunc)) {
	handle("POST /api/vmcps/{vmcp_id}/profiles/{profile_id}/components/{component_id}/oauth", h.Authorize)
	handle("DELETE /api/vmcps/{vmcp_id}/profiles/{profile_id}/components/{component_id}/oauth", h.Reset)
}

// Authorize starts the authorization of the profile's grant for the component, and returns the URL the administrator
// opens (types.VMCPProfileOAuthAuthorization). The grant is stored by the provider's callback, which only the
// administrator who started the authorization can complete. Authorizing a component that already has a grant replaces
// the grant once the new authorization completes.
func (h *VMCPProfileOAuthHandler) Authorize(req api.Context) error {
	if err := agentIdentitiesAvailable(h.enabled, h.authEnabled); err != nil {
		return err
	}
	authURL, err := h.authorizer.StartProfileOAuthGrant(req, req.PathValue("vmcp_id"), req.PathValue("profile_id"), req.PathValue("component_id"))
	if err != nil {
		return err
	}
	return req.Write(types.VMCPProfileOAuthAuthorization{AuthURL: authURL})
}

// Reset revokes the profile's grant for the component by deleting it. Every agent that used it is refused the
// component until it is authorized again; an agent whose explicit profile's grant is reset falls back to the wildcard
// profile's grant, if it holds one. It returns the profile's status.
func (h *VMCPProfileOAuthHandler) Reset(req api.Context) error {
	if err := agentIdentitiesAvailable(h.enabled, h.authEnabled); err != nil {
		return err
	}
	profileID := req.PathValue("profile_id")
	vmcp, _, err := h.grants.Reset(req.Context(), req.Storage, kclient.ObjectKey{Namespace: req.Namespace(), Name: req.PathValue("vmcp_id")}, profileID, req.PathValue("component_id"))
	if err != nil {
		return err
	}
	status, err := profileStatusWithOAuth(req, h.grants, *vmcp, profileID)
	if err != nil {
		return err
	}
	return req.Write(status)
}

// profileStatusWithOAuth reports a profile's configuration status (profileStatus) with its OAuth grants.
func profileStatusWithOAuth(req api.Context, grants *vmcpconfig.ProfileOAuthGrantStore, vmcp v1.VMCP, profileID string) (types.VMCPProfileStatus, error) {
	status := profileStatus(vmcp, profileID)
	oauth, err := grants.ProfileOAuthStatus(req.Context(), vmcp, profileID)
	if err != nil {
		return types.VMCPProfileStatus{}, err
	}
	vmcpconfig.AddOAuthStatus(&status, oauth)
	return status, nil
}
