package handlers

import (
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// PreconfiguredOAuthAuthorizer starts an administrator's authorization of a vMCP component's preconfigured OAuth. The
// MCP OAuth handler factory implements it; this interface breaks an import cycle.
type PreconfiguredOAuthAuthorizer interface {
	StartPreconfiguredOAuth(req api.Context, vmcpID, componentID string) (types.VMCPPreconfiguredOAuthAuthorization, error)
}

// VMCPPreconfiguredOAuthHandler serves the preconfigured OAuth of the remote components of shared vMCPs: one account
// per component, which an administrator authorizes once and everyone who reaches the component through the vMCP uses,
// users and agents alike. It lives outside the manifest, so it is available on catalog-synced vMCPs too.
//
// pkg/api/authz decides who reaches these routes: they are under the administrators' and owners' /api/vmcps/ rule, the
// GET is granted to read-only administrators and auditors, and no other role, group or agent is granted them. So only
// administrators and owners authorize and revoke, and read-only administrators and auditors read the state. The routes
// return the grant's metadata (who authorized it, when, for which target and scopes), never a token.
type VMCPPreconfiguredOAuthHandler struct {
	authorizer PreconfiguredOAuthAuthorizer
	grants     *vmcpconfig.PreconfiguredOAuthStore
}

func NewVMCPPreconfiguredOAuthHandler(authorizer PreconfiguredOAuthAuthorizer, grants *vmcpconfig.PreconfiguredOAuthStore) *VMCPPreconfiguredOAuthHandler {
	return &VMCPPreconfiguredOAuthHandler{
		authorizer: authorizer,
		grants:     grants,
	}
}

// RegisterRoutes registers the routes with handle. The component's .../oauth route, beside these, is a user clearing
// their own credentials for the component, which these never touch.
func (h *VMCPPreconfiguredOAuthHandler) RegisterRoutes(handle func(pattern string, f api.HandlerFunc)) {
	handle("GET /api/vmcps/{vmcp_id}/components/{component_id}/preconfigured-oauth", h.Get)
	handle("POST /api/vmcps/{vmcp_id}/components/{component_id}/preconfigured-oauth", h.Authorize)
	handle("DELETE /api/vmcps/{vmcp_id}/components/{component_id}/preconfigured-oauth", h.Revoke)
}

// Get reports the component's preconfigured OAuth (types.VMCPPreconfiguredOAuth): not required, needs authorization,
// authorized or invalid, with who authorized it and when. It is computed on the request, from the vMCP and the stored
// grant, with the checks of agent readiness, so that it agrees with every agent's readiness. A vMCP or component that
// does not exist is 404, and a personal vMCP or a component that is not a remote server is 400.
func (h *VMCPPreconfiguredOAuthHandler) Get(req api.Context) error {
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); apierrors.IsNotFound(err) {
		return types.NewErrNotFound("vMCP not found")
	} else if err != nil {
		return err
	}
	status, err := h.grants.Status(req.Context(), vmcp, req.PathValue("component_id"))
	if err != nil {
		return err
	}
	return req.Write(status)
}

// Authorize starts the authorization of the component's preconfigured OAuth, and returns the URL the administrator
// opens (types.VMCPPreconfiguredOAuthAuthorization), or, for a provider that only redirects to a localhost callback,
// the local login URL that the administrator gives `obot mcp login`. The grant is stored by the provider's callback,
// which only the administrator who started the authorization can complete. Authorizing a component that already has a
// grant replaces the grant once the new authorization completes.
func (h *VMCPPreconfiguredOAuthHandler) Authorize(req api.Context) error {
	authorization, err := h.authorizer.StartPreconfiguredOAuth(req, req.PathValue("vmcp_id"), req.PathValue("component_id"))
	if err != nil {
		return err
	}
	return req.Write(authorization)
}

// Revoke revokes the component's preconfigured OAuth by deleting its grant. Users of the vMCP go back to their own
// OAuth for the component, and every agent granted the component is refused it until it is authorized again. It is
// allowed for a component the vMCP no longer has: removing a grant can never give anyone the wrong one.
func (h *VMCPPreconfiguredOAuthHandler) Revoke(req api.Context) error {
	componentID := req.PathValue("component_id")
	vmcp, revoked, err := h.grants.Revoke(req.Context(), req.Storage, kclient.ObjectKey{Namespace: req.Namespace(), Name: req.PathValue("vmcp_id")}, componentID)
	if err != nil {
		return err
	}
	// Revoking a component that holds no grant deletes nothing, and records nothing.
	if revoked {
		recordAdminAudit(req, gclient.AdminAuditEventInput{
			Action:       gtypes.AdminAuditActionVMCPPreconfiguredOAuthRevoke,
			ResourceKind: gtypes.AdminAuditResourceVMCP,
			ResourceID:   vmcp.Name,
			Details:      map[string]any{"componentID": componentID},
		})
	}
	req.WriteHeader(http.StatusNoContent)
	return nil
}
