package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ProfileOAuthAuthorizer starts an administrator's authorization of a vMCP profile's OAuth grant. The MCP OAuth handler
// factory implements it; this interface breaks an import cycle.
type ProfileOAuthAuthorizer interface {
	StartProfileOAuthGrant(req api.Context, vmcpID, profileID, componentID string) (types.VMCPProfileOAuthAuthorization, error)
}

// VMCPProfileOAuthHandler serves the routes that report a profile's status and authorize and reset the OAuth grants a vMCP profile holds for its
// components, which every agent that resolves to the profile uses headlessly.
//
// The routes are admin-only, which the authorizer enforces: they are under the administrators' /api/vmcps/ rule, no
// other role or group is granted them, and they are not on the agents' allowlist. They return the grant's metadata
// (who authorized it, when, for which target and scopes), never a token.
type VMCPProfileOAuthHandler struct {
	authorizer ProfileOAuthAuthorizer
	grants     *vmcpconfig.ProfileOAuthGrantStore
}

func NewVMCPProfileOAuthHandler(authorizer ProfileOAuthAuthorizer, grants *vmcpconfig.ProfileOAuthGrantStore) *VMCPProfileOAuthHandler {
	return &VMCPProfileOAuthHandler{
		authorizer: authorizer,
		grants:     grants,
	}
}

// RegisterRoutes registers the routes with handle.
func (h *VMCPProfileOAuthHandler) RegisterRoutes(handle func(pattern string, f api.HandlerFunc)) {
	handle("GET /api/vmcps/{vmcp_id}/profiles/{profile_id}/status", h.Status)
	handle("POST /api/vmcps/{vmcp_id}/profiles/{profile_id}/components/{component_id}/oauth", h.Authorize)
	handle("DELETE /api/vmcps/{vmcp_id}/profiles/{profile_id}/components/{component_id}/oauth", h.Reset)
}

// Status reports how far a profile is set up for agents (types.VMCPProfileStatus): for every component of the vMCP that
// the profile grants, which configuration keys it sets and lacks, and the OAuth grant it holds.
// It describes the profile alone, with no fallback to another profile: an agent's readiness, which applies the
// fallback, is reported by the agent's routes. It is computed on the request, from the status the vMCP publishes and the
// stored grants, and returns key names, grant metadata and no value or token. A vMCP, or a profile, that does not
// exist is 404.
func (h *VMCPProfileOAuthHandler) Status(req api.Context) error {
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); apierrors.IsNotFound(err) {
		return types.NewErrNotFound("vMCP not found")
	} else if err != nil {
		return err
	}
	profileID := req.PathValue("profile_id")
	if _, ok := vmcp.Spec.Manifest.Profile(profileID); !ok {
		return types.NewErrNotFound("profile not found")
	}
	status, err := profileStatusWithOAuth(req, h.grants, vmcp, profileID)
	if err != nil {
		return err
	}
	return req.Write(status)
}

// Authorize starts the authorization of the profile's grant for the component, and returns the URL the administrator
// opens (types.VMCPProfileOAuthAuthorization), or, for a provider that only redirects to a localhost callback, the
// local login URL that the administrator gives `obot mcp login`. The grant is stored by the provider's callback, which only the
// administrator who started the authorization can complete. Authorizing a component that already has a grant replaces
// the grant once the new authorization completes.
func (h *VMCPProfileOAuthHandler) Authorize(req api.Context) error {
	authorization, err := h.authorizer.StartProfileOAuthGrant(req, req.PathValue("vmcp_id"), req.PathValue("profile_id"), req.PathValue("component_id"))
	if err != nil {
		return err
	}
	return req.Write(authorization)
}

// Reset revokes the profile's grant for the component by deleting it. Every agent that used it is refused the
// component until it is authorized again; an agent whose explicit profile's grant is reset falls back to the wildcard
// profile's grant, if it holds one. It returns the profile's status.
func (h *VMCPProfileOAuthHandler) Reset(req api.Context) error {
	profileID := req.PathValue("profile_id")
	componentID := req.PathValue("component_id")
	vmcp, deleted, err := h.grants.Reset(req.Context(), req.Storage, kclient.ObjectKey{Namespace: req.Namespace(), Name: req.PathValue("vmcp_id")}, profileID, componentID)
	if err != nil {
		return err
	}
	// Resetting a component that holds no grant deletes nothing, and records nothing.
	if deleted {
		recordAdminAudit(req, gclient.AdminAuditEventInput{
			Action:       gtypes.AdminAuditActionVMCPProfileOAuthReset,
			ResourceKind: gtypes.AdminAuditResourceVMCPProfile,
			ResourceID:   profileID,
			ParentID:     vmcp.Name,
			Details:      map[string]any{"componentID": componentID},
		})
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
