package vmcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// AgentConfigurationRequiredError is the error of the 409 response to an agent whose vMCP lacks configuration it
	// needs.
	AgentConfigurationRequiredError = "agent_configuration_required"
	// AgentOAuthRequiredError is the error of the 409 response to an agent whose vMCP lacks a working OAuth grant.
	AgentOAuthRequiredError = "agent_oauth_required"
)

// ReadinessReason is one reason an agent cannot use a vMCP. It never carries a configuration value. Its Code is
// types.AgentReadinessConfigurationRequired, types.AgentReadinessOAuthRequired or types.AgentReadinessOAuthInvalid.
type ReadinessReason = types.AgentReadinessReason

// Readiness is whether an agent can use a vMCP. Any reason blocks the whole connection: an agent is never served a
// subset of the tools it is granted.
type Readiness struct {
	// Reasons are why the agent cannot use the vMCP. There are none when it can.
	Reasons []ReadinessReason `json:"reasons,omitempty"`
}

// AgentReadinessReport is what the connect path and the readiness API learn about an agent on a vMCP: its readiness,
// and the grants it uses.
type AgentReadinessReport struct {
	Readiness Readiness
	// Grants are the vMCP's grants for the components enabled for the agent, one for each, in the order of
	// EnabledComponents (SelectGrants), with what comparing their URLs with the URLs the agent reaches found.
	Grants []ComponentOAuthGrant
}

// AgentReadinessFailure is the body of the 409 response to an agent that cannot use a vMCP.
type AgentReadinessFailure struct {
	Error   string            `json:"error"`
	Reasons []ReadinessReason `json:"reasons"`
}

// AgentNotReadyError reports that an agent cannot use a vMCP, so that the MCP gateway answers it with a 409 rather
// than an interactive challenge.
type AgentNotReadyError struct {
	Readiness Readiness
}

// Ready reports whether nothing keeps the agent from using the vMCP.
func (r Readiness) Ready() bool {
	return len(r.Reasons) == 0
}

// ErrorCode returns the error of the 409 response for the readiness. A mix of reasons is reported by the one an
// administrator fixes first: AgentConfigurationRequiredError if any reason is not about OAuth, because the vMCP's fixed
// configuration can decide which upstream a grant is for; and AgentOAuthRequiredError when every reason is about OAuth.
func (r Readiness) ErrorCode() string {
	if len(r.Reasons) > 0 && !slices.ContainsFunc(r.Reasons, func(reason ReadinessReason) bool {
		return reason.Code != types.AgentReadinessOAuthRequired && reason.Code != types.AgentReadinessOAuthInvalid
	}) {
		return AgentOAuthRequiredError
	}
	return AgentConfigurationRequiredError
}

func (e *AgentNotReadyError) Error() string {
	codes := make([]string, 0, len(e.Readiness.Reasons))
	for _, reason := range e.Readiness.Reasons {
		codes = append(codes, reason.ComponentID+": "+reason.Code)
	}
	return fmt.Sprintf("%s: %s", e.Readiness.ErrorCode(), strings.Join(codes, ", "))
}

// Failure returns the body of the 409 response.
func (e *AgentNotReadyError) Failure() AgentReadinessFailure {
	return AgentReadinessFailure{Error: e.Readiness.ErrorCode(), Reasons: e.Readiness.Reasons}
}

// AgentReadiness reports whether agent can use vmcp through instance, its connection to it, or nil when it has not
// connected, where grants is the store of agent OAuth grants. The connect path and the readiness API both call it,
// with the same store, so they never disagree. It only checks the components enabled for the agent: the union of the
// components that the profiles naming it grant (EnabledComponents), as for a user. It is computed anew on every call
// from the vMCP's current state and the stored grants. It does not check access: an agent that no profile names has
// no components enabled, and the caller must refuse it.
//
// A component is not ready while it needs a value that agents cannot be given, because neither the vMCP nor the
// catalog entry fixes it (AgentMissingConfiguration), or while the vMCP's grant for it does not work, or it holds none
// for a component known to need OAuth (ComponentOAuthGrant.ReadinessReasons). The grants are selected for every
// component at once (SelectGrants), and each usable one is compared with the URL the agent reaches (checkGrantTarget).
// The vMCP's agent setup (AgentSetup) reports the same reasons for each component.
func AgentReadiness(ctx context.Context, grants *AgentOAuthGrantStore, vmcp v1.VMCP, instance *v1.VMCPInstance, agent kuser.Info) (AgentReadinessReport, error) {
	if !principal.IsAgent(agent) {
		return AgentReadinessReport{}, fmt.Errorf("agent readiness is only for agents")
	}
	if vmcp.Spec.UserID != "" {
		return AgentReadinessReport{}, fmt.Errorf("VMCP %q is personal, and agents only use shared VMCPs", vmcp.Name)
	}
	if grants == nil {
		return AgentReadinessReport{}, fmt.Errorf("agent readiness needs the store of vMCP agent OAuth grants")
	}
	var current v1.VMCPInstance
	if instance != nil {
		current = *instance
	}
	var report AgentReadinessReport
	components := EnabledComponents(agent, vmcp, ComponentsForInstance(vmcp, current))
	var err error
	if report.Grants, err = grants.SelectGrants(ctx, vmcp, components); err != nil {
		return AgentReadinessReport{}, err
	}
	for i, component := range components {
		reasons, err := grants.componentReasons(ctx, vmcp, component, &report.Grants[i])
		if err != nil {
			return AgentReadinessReport{}, fmt.Errorf("check component %q for agent readiness: %w", component.ID, err)
		}
		report.Readiness.Reasons = append(report.Readiness.Reasons, reasons...)
	}
	return report, nil
}

// AgentSetup reports what agents need from vmcp (types.VMCPAgentSetup): for each component that a profile naming an
// agent grants (AgentGrantedComponents), whether it keeps the agents granted it from the vMCP, and why, with the
// checks of AgentReadiness, and the vMCP's grant for it. Every agent granted a component that is not ready is not
// ready, and an agent granted only ready components is. It never reads or returns a value or a token.
func AgentSetup(ctx context.Context, grants *AgentOAuthGrantStore, vmcp v1.VMCP) (types.VMCPAgentSetup, error) {
	if grants == nil {
		return types.VMCPAgentSetup{}, fmt.Errorf("agent setup needs the store of vMCP agent OAuth grants")
	}
	components := AgentGrantedComponents(vmcp)
	selected, err := grants.SelectGrants(ctx, vmcp, components)
	if err != nil {
		return types.VMCPAgentSetup{}, err
	}
	setup := types.VMCPAgentSetup{VMCPID: vmcp.Name, Components: make([]types.VMCPAgentSetupComponent, 0, len(components))}
	for i, component := range components {
		reasons, err := grants.componentReasons(ctx, vmcp, component, &selected[i])
		if err != nil {
			return types.VMCPAgentSetup{}, fmt.Errorf("check component %q for agent setup: %w", component.ID, err)
		}
		item := types.VMCPAgentSetupComponent{
			ComponentID: component.ID,
			Ready:       len(reasons) == 0,
			Reasons:     append([]types.AgentReadinessReason{}, reasons...),
		}
		if ComponentUsesOAuth(component) {
			status := selected[i].Status(component)
			item.OAuth = &status
		}
		setup.Components = append(setup.Components, item)
	}
	return setup, nil
}
