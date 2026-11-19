package vmcp

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
)

// ReconcileProfiles gives each profile of a vMCP stored before profiles had IDs its derived ID, once. Profiles hold no
// data of their own, so a profile's removal needs nothing else.
func (h *Handler) ReconcileProfiles(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)
	if !vmcp.DeletionTimestamp.IsZero() {
		return nil
	}
	// The API and catalog sync assign IDs when they save a vMCP, so only a vMCP stored before profiles had IDs lacks
	// them. Its profiles get the IDs catalog sync derives, so a catalog-synced vMCP keeps them on its next sync.
	if vmcpconfig.AssignDerivedProfileIDs(vmcp.Name, &vmcp.Spec.Manifest) {
		return req.Client.Update(req.Ctx, vmcp)
	}
	return nil
}

// CollectAgentOAuthGrants deletes the agent OAuth grants of components the vMCP no longer has, by the records of the
// grants in status, which every grant is recorded in before it is stored (vmcpconfig.AgentOAuthGrantStore.Authorize).
func (h *Handler) CollectAgentOAuthGrants(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)
	if !vmcp.DeletionTimestamp.IsZero() || h.agentOAuthGrants == nil || len(vmcp.Status.AgentOAuthGrants) == 0 {
		// The finalizer deletes every grant of a vMCP being deleted.
		return nil
	}
	changed, err := h.agentOAuthGrants.DeleteRemovedComponentGrants(req.Ctx, vmcp)
	if err != nil || !changed {
		return err
	}
	return req.Client.Status().Update(req.Ctx, vmcp)
}

// RemoveAgentOAuthGrants deletes every agent OAuth grant of a vMCP being deleted. It runs in the vMCP's finalizer, and
// can run again after any failure.
func (h *Handler) RemoveAgentOAuthGrants(req router.Request, _ router.Response) error {
	if h.agentOAuthGrants == nil {
		return nil
	}
	return h.agentOAuthGrants.DeleteVMCPGrants(req.Ctx, *req.Object.(*v1.VMCP))
}
