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

// AgentOAuthAuthorizer starts an administrator's authorization of a vMCP's agent OAuth grant. The MCP OAuth handler
// factory implements it; this interface breaks an import cycle.
type AgentOAuthAuthorizer interface {
	StartAgentOAuthGrant(req api.Context, vmcpID, componentID string) (types.VMCPAgentOAuthAuthorization, error)
}

// VMCPAgentOAuthHandler serves the routes of a shared vMCP's agent setup: what agents need from the vMCP, and the agent
// OAuth grants the vMCP holds for its components, one per component, which every agent granted the component uses
// headlessly. Profiles hold nothing for agents; these routes are the vMCP's.
//
// The routes are admin-only, which the authorizer enforces: they are under the administrators' /api/vmcps/ rule, no
// other role or group is granted them, except that auditors read the agent setup, and they are not on the agents'
// allowlist. They return the grants' metadata (who authorized each, when, for which target and scopes), never a token.
type VMCPAgentOAuthHandler struct {
	authorizer AgentOAuthAuthorizer
	grants     *vmcpconfig.AgentOAuthGrantStore
}

func NewVMCPAgentOAuthHandler(authorizer AgentOAuthAuthorizer, grants *vmcpconfig.AgentOAuthGrantStore) *VMCPAgentOAuthHandler {
	return &VMCPAgentOAuthHandler{
		authorizer: authorizer,
		grants:     grants,
	}
}

// RegisterRoutes registers the routes with handle.
func (h *VMCPAgentOAuthHandler) RegisterRoutes(handle func(pattern string, f api.HandlerFunc)) {
	handle("GET /api/vmcps/{vmcp_id}/agent-setup", h.Setup)
	handle("POST /api/vmcps/{vmcp_id}/agent-oauth/{component_id}", h.Authorize)
	handle("DELETE /api/vmcps/{vmcp_id}/agent-oauth/{component_id}", h.Revoke)
}

// Setup reports what agents need from the vMCP (types.VMCPAgentSetup): for every component that a profile naming an
// agent grants, the configuration agents need and cannot be given, and the OAuth grant the vMCP holds, with the checks
// of agent readiness, so that it agrees with every agent's readiness. It is computed on the request, from the vMCP and
// the stored grants, and returns key names, grant metadata and no value or token. A vMCP that does not exist is 404.
func (h *VMCPAgentOAuthHandler) Setup(req api.Context) error {
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); apierrors.IsNotFound(err) {
		return types.NewErrNotFound("vMCP not found")
	} else if err != nil {
		return err
	}
	setup, err := vmcpconfig.AgentSetup(req.Context(), h.grants, vmcp)
	if err != nil {
		return err
	}
	return req.Write(setup)
}

// Authorize starts the authorization of the vMCP's grant for the component, and returns the URL the administrator
// opens (types.VMCPAgentOAuthAuthorization), or, for a provider that only redirects to a localhost callback, the local
// login URL that the administrator gives `obot mcp login`. The grant is stored by the provider's callback, which only
// the administrator who started the authorization can complete. Authorizing a component that already has a grant
// replaces the grant once the new authorization completes.
func (h *VMCPAgentOAuthHandler) Authorize(req api.Context) error {
	authorization, err := h.authorizer.StartAgentOAuthGrant(req, req.PathValue("vmcp_id"), req.PathValue("component_id"))
	if err != nil {
		return err
	}
	return req.Write(authorization)
}

// Revoke revokes the vMCP's grant for the component by deleting it. Every agent granted the component is refused it
// until it is authorized again. It returns the vMCP's agent setup.
func (h *VMCPAgentOAuthHandler) Revoke(req api.Context) error {
	componentID := req.PathValue("component_id")
	vmcp, revoked, err := h.grants.Revoke(req.Context(), req.Storage, kclient.ObjectKey{Namespace: req.Namespace(), Name: req.PathValue("vmcp_id")}, componentID)
	if err != nil {
		return err
	}
	// Revoking a component that holds no grant deletes nothing, and records nothing.
	if revoked {
		recordAdminAudit(req, gclient.AdminAuditEventInput{
			Action:       gtypes.AdminAuditActionVMCPAgentOAuthRevoke,
			ResourceKind: gtypes.AdminAuditResourceVMCP,
			ResourceID:   vmcp.Name,
			Details:      map[string]any{"componentID": componentID},
		})
	}
	setup, err := vmcpconfig.AgentSetup(req.Context(), h.grants, *vmcp)
	if err != nil {
		return err
	}
	return req.Write(setup)
}
