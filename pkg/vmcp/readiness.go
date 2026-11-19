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
	// ProfileID is the profile that names the agent (AgentProfile), if any.
	ProfileID string `json:"profileID,omitempty"`
	// Reasons are why the agent cannot use the vMCP. There are none when it can.
	Reasons []ReadinessReason `json:"reasons,omitempty"`
}

// AgentReadinessReport is what the readiness API reports about an agent on a vMCP: the readiness that the connect path
// computes, and where the agent's configuration comes from.
type AgentReadinessReport struct {
	Readiness Readiness
	// Configuration is how the agent's profiles supply its configuration (ResolveAgentConfiguration). It names keys,
	// never values.
	Configuration AgentConfiguration
	// Grants are the grants the agent uses, one for each component enabled for it, in the order of EnabledComponents
	// (SelectAgentGrants), with what comparing their URLs with the URLs the agent reaches found.
	Grants []AgentOAuthGrant
	// PendingSync reports that the agent's instance exists and has not yet been given the configuration its profiles now
	// resolve to, so the next connect waits for the vMCP instance controller.
	PendingSync bool
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
// administrator fixes first: AgentConfigurationRequiredError if any reason is not about OAuth, because configuration
// can decide which upstream a grant is for; and AgentOAuthRequiredError when every reason is about OAuth.
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

// AgentProfile returns the profile of manifest that names agent, and nil when none does. An agent matches only a
// subject that names it by ID, and rule 2 (types.VMCPManifest.Validate) allows at most one profile to name it, so this
// is the only profile that grants the agent anything on the vMCP, and the only one whose configuration and OAuth
// grants it uses.
func AgentProfile(agent kuser.Info, manifest types.VMCPManifest) *types.VMCPProfile {
	if !principal.IsAgent(agent) {
		return nil
	}
	matching := MatchingProfiles(agent, manifest.Profiles)
	if len(matching) == 0 {
		return nil
	}
	return &matching[0]
}

// profileForComponent returns profile when it grants the component with componentID (ProfileGrantsComponent), and nil
// otherwise. A profile's configuration and OAuth grants apply to a component only while the profile grants it, which
// is also the only time an administrator can set them up there. Values or grants that a profile kept from when it
// granted the component stay stored, and are used again only if it grants the component again.
func profileForComponent(profile *types.VMCPProfile, componentID string) *types.VMCPProfile {
	if profile == nil || !ProfileGrantsComponent(*profile, componentID) {
		return nil
	}
	return profile
}

// AgentReadiness reports whether agent can use vmcp through instance, its connection to it, or nil when it has not
// connected, where grants is the store of profile OAuth grants. The connect path and the readiness API both call it,
// with the same store, so they never disagree. It only checks the components enabled for the agent, and it is computed
// anew on every call from the vMCP's current state, the keys its profiles' configuration revisions publish, and the
// stored grants. It does not check access: an agent that no profile names has no components enabled, and the caller
// must refuse it.
//
// A component takes what it needs from the agent's profile (AgentProfile). It is not ready while a required value that
// a user would enter on their own connection is not supplied by the profile (configurationReasons), or while the grant
// the agent would use does not work, or none is held for a component known to need OAuth
// (AgentOAuthGrant.ReadinessReasons). The grants are selected for every component at once (SelectAgentGrants), and
// each usable one is compared with the URL the agent reaches (checkGrantTarget).
func AgentReadiness(ctx context.Context, grants *ProfileOAuthGrantStore, vmcp v1.VMCP, instance *v1.VMCPInstance, agent kuser.Info) (AgentReadinessReport, error) {
	if !principal.IsAgent(agent) {
		return AgentReadinessReport{}, fmt.Errorf("agent readiness is only for agents")
	}
	if vmcp.Spec.UserID != "" {
		return AgentReadinessReport{}, fmt.Errorf("VMCP %q is personal, and agents only use shared VMCPs", vmcp.Name)
	}
	if grants == nil {
		return AgentReadinessReport{}, fmt.Errorf("agent readiness needs the store of vMCP profile OAuth grants")
	}
	var current v1.VMCPInstance
	if instance != nil {
		current = *instance
	}
	report := AgentReadinessReport{Configuration: ResolveAgentConfiguration(vmcp, current, agent)}
	report.PendingSync = instance != nil && instance.Status.AgentConfigurationRevision != report.Configuration.Revision

	profile := AgentProfile(agent, vmcp.Spec.Manifest)
	readiness := &report.Readiness
	if profile != nil {
		readiness.ProfileID = profile.ID
	}

	// ResolveAgentConfiguration resolved these same components, in the same order.
	components := EnabledComponents(agent, vmcp, ComponentsForInstance(vmcp, current))
	var err error
	if report.Grants, err = grants.SelectAgentGrants(ctx, vmcp, profile, components); err != nil {
		return AgentReadinessReport{}, err
	}
	// Every enabled component is one the agent's profile grants, since no other profile names the agent.
	for i, component := range components {
		// The MCP gateway refuses a grant for another URL than the agent's connection reaches; readiness says so first.
		if err := grants.checkGrantTarget(ctx, vmcp, profile, component, &report.Grants[i]); err != nil {
			return AgentReadinessReport{}, fmt.Errorf("check component %q for agent readiness: %w", component.ID, err)
		}
		readiness.Reasons = append(readiness.Reasons, configurationReasons(report.Configuration.Components[i], readiness.ProfileID)...)
		readiness.Reasons = append(readiness.Reasons, report.Grants[i].ReadinessReasons(component, readiness.ProfileID)...)
	}
	return report, nil
}
