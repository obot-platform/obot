package handlers

import (
	"context"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// AgentReadinessSource decides whether an agent can connect to a vMCP. The MCP session manager, whose connect path
// refuses an agent that is not ready, implements it, so that the readiness API is computed as that connect path
// computes it, with the same store of agent OAuth grants.
type AgentReadinessSource interface {
	AgentReadiness(ctx context.Context, vmcp v1.VMCP, instance *v1.VMCPInstance, agent kuser.Info) (vmcpconfig.AgentReadinessReport, error)
}

// ListVMCPReadiness lists the vMCPs an agent can access, with whether it can use each and why not. It is computed on
// the request from the vMCPs' current state, and nothing it reads is a cached summary.
//
// It costs one list of vMCPs and one list of the agent's instances, then, for each vMCP, the readiness checks:
// configuration is checked against the vMCP's manifest, so costs nothing, and the vMCP's grants for the components
// that can hold an OAuth grant cost one query of the gateway database (plus the credential read that resolves a URL the
// vMCP fixes, for a component whose URL comes from configuration). A vMCP no profile names the agent for is not listed.
func (h *AgentIdentityHandler) ListVMCPReadiness(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}
	info := agentInfo(agent)

	vmcps, err := listSharedVMCPs(req)
	if err != nil {
		return err
	}
	instances, err := agentInstances(req, agent)
	if err != nil {
		return err
	}

	items := []types.AgentVMCPReadiness{}
	for _, vmcp := range vmcps {
		if len(vmcpconfig.MatchingProfiles(info, vmcp.Spec.Manifest.Profiles)) == 0 {
			continue
		}
		item, err := h.vmcpReadiness(req, vmcp, instances[vmcp.Name], info, agent.Status() == types.UserStatusDisabled)
		if err != nil {
			return err
		}
		items = append(items, item)
	}
	return req.Write(types.AgentVMCPReadinessList{Items: items})
}

// GetVMCPReadiness returns whether an agent can use one vMCP, and why not. A vMCP that does not exist, is personal, is being deleted, or that no profile names the agent for is 404, as
// it is not among the vMCPs the agent can access.
func (h *AgentIdentityHandler) GetVMCPReadiness(req api.Context) error {
	agent, err := pathAgent(req)
	if err != nil {
		return err
	}
	info := agentInfo(agent)

	var vmcp v1.VMCP
	if err := req.Get(&vmcp, req.PathValue("vmcp_id")); apierrors.IsNotFound(err) {
		return types.NewErrNotFound("vMCP not found")
	} else if err != nil {
		return err
	}
	if vmcp.IsPersonal() || !vmcp.DeletionTimestamp.IsZero() || len(vmcpconfig.MatchingProfiles(info, vmcp.Spec.Manifest.Profiles)) == 0 {
		return types.NewErrNotFound("the agent does not have access to this vMCP")
	}
	instances, err := agentInstances(req, agent)
	if err != nil {
		return err
	}

	item, err := h.vmcpReadiness(req, vmcp, instances[vmcp.Name], info, agent.Status() == types.UserStatusDisabled)
	if err != nil {
		return err
	}
	return req.Write(item)
}

// vmcpReadiness computes whether agent can use vmcp, with the checks of the connect path.
func (h *AgentIdentityHandler) vmcpReadiness(req api.Context, vmcp v1.VMCP, instance *v1.VMCPInstance, agent kuser.Info, disabled bool) (types.AgentVMCPReadiness, error) {
	report, err := h.readiness.AgentReadiness(req.Context(), vmcp, instance, agent)
	if err != nil {
		return types.AgentVMCPReadiness{}, fmt.Errorf("compute readiness of agent %s for vMCP %q: %w", agent.GetUID(), vmcp.Name, err)
	}
	return types.AgentVMCPReadiness{
		VMCPID:          vmcp.Name,
		VMCPDisplayName: vmcp.Spec.Manifest.DisplayName,
		Ready:           report.Readiness.Ready(),
		Disabled:        disabled,
		Reasons:         append([]types.AgentReadinessReason{}, report.Readiness.Reasons...),
	}, nil
}

// agentInfo is the caller the policy matchers and the readiness checks evaluate for agent. An agent is in no group,
// and the checks see only what its profiles name it by.
func agentInfo(agent *gtypes.Agent) kuser.Info {
	return gclient.AgentUserInfo(agent, nil, nil)
}

// agentInstances returns the agent's instance of each vMCP it has connected to, by vMCP ID, with one list. The
// instance is the agent's own: instances are stored under the encoding of its principal, never under a user's ID.
func agentInstances(req api.Context, agent *gtypes.Agent) (map[string]*v1.VMCPInstance, error) {
	var instances v1.VMCPInstanceList
	if err := req.List(&instances, kclient.MatchingFields{"spec.userID": principal.Agent(agent.ID).String()}); err != nil {
		return nil, fmt.Errorf("failed to list instances of agent %d: %w", agent.ID, err)
	}
	byVMCP := make(map[string]*v1.VMCPInstance, len(instances.Items))
	for i := range instances.Items {
		if instances.Items[i].DeletionTimestamp.IsZero() {
			byVMCP[instances.Items[i].Spec.Manifest.VMCPID] = &instances.Items[i]
		}
	}
	return byVMCP, nil
}
