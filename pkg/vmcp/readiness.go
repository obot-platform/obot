package vmcp

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// AgentSelectionAmbiguousError is the error of the 409 response to an agent that needs the profile it falls back
	// to, on a vMCP where that profile is ambiguous. Only an administrator can fix it, by leaving one profile that names
	// every agent.
	AgentSelectionAmbiguousError = "agent_selection_ambiguous"
	// AgentConfigurationRequiredError is the error of the 409 response to an agent whose vMCP lacks configuration it
	// needs.
	AgentConfigurationRequiredError = "agent_configuration_required"
	// AgentOAuthRequiredError is the error of the 409 response to an agent whose vMCP lacks a working OAuth grant.
	AgentOAuthRequiredError = "agent_oauth_required"
)

// ReadinessReason is one reason an agent cannot use a vMCP. It never carries a configuration value. Its Code is
// types.AgentReadinessConfigurationRequired, types.AgentReadinessOAuthRequired, types.AgentReadinessOAuthInvalid or
// types.AgentReadinessSelectionAmbiguous.
type ReadinessReason = types.AgentReadinessReason

// Readiness is whether an agent can use a vMCP. Any reason blocks the whole connection: an agent is never served a
// subset of the tools it is granted.
type Readiness struct {
	// ExplicitProfileID is the profile that names the agent by ID, if any.
	ExplicitProfileID string `json:"explicitProfileID,omitempty"`
	// WildcardProfileID is the profile that names every agent, if exactly one does.
	WildcardProfileID string `json:"wildcardProfileID,omitempty"`
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
	// (SelectAgentGrants), with what comparing their URLs with the URLs the agent reaches found. They are nil when the
	// agent's fallback is ambiguous and it has no explicit profile, as nothing else is checked then.
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

// AgentSelection is how a vMCP's profiles apply to an agent.
type AgentSelection struct {
	// Explicit is the profile that names the agent by ID. Rule 2 (types.VMCPManifest.Validate) allows at most one.
	Explicit *types.VMCPProfile
	// Wildcard is the profile that names every agent, which the agent falls back to. It is nil when no profile, or
	// more than one, names every agent.
	Wildcard *types.VMCPProfile
	// Ambiguous reports that more than one profile names every agent, as a vMCP stored before the wildcard rule may
	// (types.VMCPManifest.AgentWildcardProfiles), so the fallback is unknown and Wildcard is nil. An agent with an
	// explicit profile is still checked: only what it would need from the fallback is ambiguous (AgentReadiness).
	Ambiguous bool
}

// Ready reports whether nothing keeps the agent from using the vMCP.
func (r Readiness) Ready() bool {
	return len(r.Reasons) == 0
}

// ErrorCode returns the error of the 409 response for the readiness. A mix of reasons is reported by the one an
// administrator fixes first: AgentSelectionAmbiguousError if any reason is types.AgentReadinessSelectionAmbiguous,
// because which profile's configuration and grants apply is unknown until the profiles are fixed; then
// AgentConfigurationRequiredError if any reason is not about OAuth, because configuration can decide which upstream a
// grant is for; and AgentOAuthRequiredError when every reason is about OAuth.
func (r Readiness) ErrorCode() string {
	switch {
	case slices.ContainsFunc(r.Reasons, func(reason ReadinessReason) bool { return reason.Code == types.AgentReadinessSelectionAmbiguous }):
		return AgentSelectionAmbiguousError
	case len(r.Reasons) > 0 && !slices.ContainsFunc(r.Reasons, func(reason ReadinessReason) bool {
		return reason.Code != types.AgentReadinessOAuthRequired && reason.Code != types.AgentReadinessOAuthInvalid
	}):
		return AgentOAuthRequiredError
	default:
		return AgentConfigurationRequiredError
	}
}

func (e *AgentNotReadyError) Error() string {
	codes := make([]string, 0, len(e.Readiness.Reasons))
	for _, reason := range e.Readiness.Reasons {
		if reason.ComponentID != "" {
			codes = append(codes, reason.ComponentID+": "+reason.Code)
		} else {
			codes = append(codes, reason.Code)
		}
	}
	return fmt.Sprintf("%s: %s", e.Readiness.ErrorCode(), strings.Join(codes, ", "))
}

// Failure returns the body of the 409 response.
func (e *AgentNotReadyError) Failure() AgentReadinessFailure {
	return AgentReadinessFailure{Error: e.Readiness.ErrorCode(), Reasons: e.Readiness.Reasons}
}

// SelectAgentProfiles returns how the profiles of manifest apply to agent. Ambiguity is read from the manifest itself
// rather than from the AgentSelectionAmbiguous condition, which the vMCP controller can record late.
func SelectAgentProfiles(agent kuser.Info, manifest types.VMCPManifest) AgentSelection {
	var selection AgentSelection
	for _, match := range MatchingProfileStrengths(agent, manifest.Profiles) {
		if match.Match == subjects.MatchExplicit && selection.Explicit == nil {
			profile := match.Profile
			selection.Explicit = &profile
		}
	}
	switch wildcards := manifest.AgentWildcardProfiles(); len(wildcards) {
	case 0:
	case 1:
		selection.Wildcard = &wildcards[0]
	default:
		selection.Ambiguous = true
	}
	return selection
}

// fixProfileID is the profile where an administrator fixes a reason: the explicit profile if there is one, otherwise
// the wildcard. It is empty when the agent has no explicit profile and the wildcard is ambiguous; readiness reports
// AmbiguousFallbackReason, without a profile, for what it would need from such a fallback.
func (s AgentSelection) fixProfileID() string {
	if s.Explicit != nil {
		return s.Explicit.ID
	}
	if s.Wildcard != nil {
		return s.Wildcard.ID
	}
	return ""
}

// AmbiguousFallbackReason is the reason readiness reports for a component that needs something the agent's explicit
// profile does not supply, when the profile it falls back to is ambiguous. Unlike the
// types.AgentReadinessSelectionAmbiguous reason about the whole vMCP, it names the component: the agent has an explicit
// profile, but the component needs something only the fallback could supply.
func AmbiguousFallbackReason(componentID string) ReadinessReason {
	return ReadinessReason{ComponentID: componentID, Code: types.AgentReadinessSelectionAmbiguous}
}

// AgentReadiness reports whether agent can use vmcp through instance, its connection to it, or nil when it has not
// connected, where grants is the store of profile OAuth grants. The connect path and the readiness API both call it,
// with the same store, so they never disagree. It only checks the components enabled for the agent, and it is computed
// anew on every call from the vMCP's current state, the keys its profiles' configuration revisions publish, and the
// stored grants. It does not check access: an agent that no profile names has no components enabled, and the caller
// must refuse it.
//
// A component takes what it needs from the agent's explicit profile first, and only then from the wildcard profile the
// agent falls back to. It is not ready while a required value that a user would enter on their own connection is
// supplied by neither (configurationReasons), or while the grant the agent would use does not work, or none is held
// for a component known to need OAuth (AgentOAuthGrant.ReadinessReasons). The grants are selected for every component
// at once (SelectAgentGrants), and each usable one is compared with the URL the agent reaches (checkGrantTarget).
//
// When more than one profile names every agent, an agent without an explicit profile would take all its setup from
// the ambiguous fallback, so it is refused with one types.AgentReadinessSelectionAmbiguous reason for the whole vMCP,
// and nothing else is checked. An agent with an explicit profile is checked component by component, and a component
// that needs something only the fallback could supply is AmbiguousFallbackReason. Tool permissions are unaffected either
// way: they are the union of every profile that names the agent.
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

	selection := SelectAgentProfiles(agent, vmcp.Spec.Manifest)
	readiness := &report.Readiness
	if selection.Explicit != nil {
		readiness.ExplicitProfileID = selection.Explicit.ID
	}
	if selection.Wildcard != nil {
		readiness.WildcardProfileID = selection.Wildcard.ID
	}
	if selection.Ambiguous && selection.Explicit == nil {
		// Every configuration value and grant would come from the fallback, which is unknown, so nothing else is
		// checked.
		readiness.Reasons = []ReadinessReason{{Code: types.AgentReadinessSelectionAmbiguous}}
		return report, nil
	}

	// ResolveAgentConfiguration resolved these same components, in the same order.
	components := EnabledComponents(agent, vmcp, ComponentsForInstance(vmcp, current))
	var err error
	if report.Grants, err = grants.SelectAgentGrants(ctx, vmcp, selection, components); err != nil {
		return AgentReadinessReport{}, err
	}
	for i, component := range components {
		// The MCP gateway refuses a grant for another URL than the agent's connection reaches; readiness says so first.
		if err := grants.checkGrantTarget(ctx, vmcp, selection, component, &report.Grants[i]); err != nil {
			return AgentReadinessReport{}, fmt.Errorf("check component %q for agent readiness: %w", component.ID, err)
		}
		// A component that needs both configuration and a grant from an ambiguous fallback is reported once.
		for _, reason := range slices.Concat(configurationReasons(report.Configuration.Components[i], selection), report.Grants[i].ReadinessReasons(component, selection)) {
			if !slices.Contains(readiness.Reasons, reason) {
				readiness.Reasons = append(readiness.Reasons, reason)
			}
		}
	}
	return report, nil
}
