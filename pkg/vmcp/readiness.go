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
	// ReadinessConfigurationRequired is a reason an agent cannot use a vMCP: a component enabled for it requires a
	// configuration value that the agent's profiles do not supply. Field names the key.
	ReadinessConfigurationRequired = "configuration_required"
	// ReadinessOAuthRequired is a reason an agent cannot use a vMCP: a component enabled for it requires OAuth, and
	// the agent's profiles hold no grant for it.
	ReadinessOAuthRequired = "oauth_required"
	// ReadinessOAuthInvalid is a reason an agent cannot use a vMCP: the grant its profile holds for a component no
	// longer works, and must be authorized again.
	ReadinessOAuthInvalid = "oauth_invalid"
	// ReadinessSelectionAmbiguous is a reason an agent cannot use a vMCP: more than one profile names every agent
	// (VMCPConditionAgentSelectionAmbiguous), so the profile an agent falls back to is unknown. Without a component, it
	// is about the whole vMCP: the agent has no explicit profile, so every setup would come from the fallback. With a
	// component, the agent has an explicit profile, but that component needs something only the fallback could supply.
	ReadinessSelectionAmbiguous = "selection_ambiguous"

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

// ReadinessReason is one reason an agent cannot use a vMCP. It never carries a configuration value.
type ReadinessReason struct {
	// ComponentID is the component the reason is about. It is empty for a reason about the whole vMCP, such as
	// ReadinessSelectionAmbiguous.
	ComponentID string `json:"componentID,omitempty"`
	// Code is one of ReadinessConfigurationRequired, ReadinessOAuthRequired, ReadinessOAuthInvalid and
	// ReadinessSelectionAmbiguous.
	Code string `json:"code"`
	// Field is the configuration key that is missing, never its value.
	Field string `json:"field,omitempty"`
	// ProfileID is the profile where an administrator fixes the reason: the agent's explicit profile if it has one,
	// otherwise the wildcard profile it falls back to. It is empty for ReadinessSelectionAmbiguous, which is fixed by
	// leaving one wildcard profile rather than in any profile's setup.
	ProfileID string `json:"profileID,omitempty"`
}

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
	// explicit profile is still checked: only what it would need from the fallback is ambiguous
	// (ComponentReadinessCheck).
	Ambiguous bool
	// Profiles are every profile that names the agent, in manifest order. Their permissions add up.
	Profiles []types.VMCPProfile
}

// AgentReadinessInput is what a ComponentReadinessCheck evaluates a component for.
type AgentReadinessInput struct {
	VMCP v1.VMCP
	// Instance is the agent's connection to the vMCP. It is the zero instance when the agent has not connected.
	Instance  v1.VMCPInstance
	Agent     kuser.Info
	Selection AgentSelection
}

// ComponentReadinessCheck is one source of reasons an agent cannot use a component enabled for it. Each source is
// replaced on its own: ProfileConfigurationCheck resolves profile configuration, and ProfileOAuthGrantCheck selects
// profile OAuth grants.
//
// A check resolves what the component needs from the agent's explicit profile first (in.Selection.Explicit), and
// only then from the profile the agent falls back to (in.Selection.Wildcard). When it would need the fallback and the
// fallback is ambiguous (in.Selection.Ambiguous), it reports AmbiguousFallbackReason for the component instead of its
// own reason, because no one can tell which profile's setup would apply. A component that the explicit profile fully
// supplies, or that needs nothing, has no reason. A check is never called for an ambiguous vMCP when the agent has no
// explicit profile: AgentReadiness refuses that agent before any component is checked.
type ComponentReadinessCheck interface {
	ComponentReadiness(ctx context.Context, in AgentReadinessInput, component types.VMCPComponent) ([]ReadinessReason, error)
}

// AgentReadinessChecks computes agent readiness from one check per source of reasons. A nil check uses its default:
// ProfileConfigurationCheck and UnsupportedAgentOAuth. The OAuth check that reads profile grants needs their store, so
// the server sets ProfileOAuthGrantCheck, and the readiness API must use the same checks as the connect path.
type AgentReadinessChecks struct {
	Configuration ComponentReadinessCheck
	OAuth         ComponentReadinessCheck
}

// UnsupportedAgentOAuth is the OAuth check without a store of profile OAuth grants (ProfileOAuthGrantCheck): a
// component that is known to require OAuth, a remote server with administrator static OAuth, has no grant for an
// agent. On a vMCP whose fallback is ambiguous, the component is reported as AmbiguousFallbackReason rather than
// ReadinessOAuthRequired. A component that only asks for OAuth when it is called is caught then, by the MCP gateway's
// headless challenge handling.
type UnsupportedAgentOAuth struct{}

// Ready reports whether nothing keeps the agent from using the vMCP.
func (r Readiness) Ready() bool {
	return len(r.Reasons) == 0
}

// ErrorCode returns the error of the 409 response for the readiness. A mix of reasons is reported by the one an
// administrator fixes first: AgentSelectionAmbiguousError if any reason is ReadinessSelectionAmbiguous, because which
// profile's configuration and grants apply is unknown until the profiles are fixed; then
// AgentConfigurationRequiredError if any reason is not about OAuth, because configuration can decide which upstream a
// grant is for; and AgentOAuthRequiredError when every reason is about OAuth.
func (r Readiness) ErrorCode() string {
	switch {
	case slices.ContainsFunc(r.Reasons, func(reason ReadinessReason) bool { return reason.Code == ReadinessSelectionAmbiguous }):
		return AgentSelectionAmbiguousError
	case len(r.Reasons) > 0 && !slices.ContainsFunc(r.Reasons, func(reason ReadinessReason) bool {
		return reason.Code != ReadinessOAuthRequired && reason.Code != ReadinessOAuthInvalid
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
		selection.Profiles = append(selection.Profiles, match.Profile)
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
// the wildcard. It is empty when the agent has no explicit profile and the wildcard is ambiguous; a check reports
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

// AmbiguousFallbackReason is the reason a ComponentReadinessCheck reports for a component that needs something the
// agent's explicit profile does not supply, when the profile it falls back to is ambiguous.
func AmbiguousFallbackReason(componentID string) ReadinessReason {
	return ReadinessReason{ComponentID: componentID, Code: ReadinessSelectionAmbiguous}
}

// AgentReadiness returns whether agent can use vmcp, with the default checks.
func AgentReadiness(ctx context.Context, vmcp v1.VMCP, instance v1.VMCPInstance, agent kuser.Info) (Readiness, error) {
	return AgentReadinessChecks{}.AgentReadiness(ctx, vmcp, instance, agent)
}

// AgentReadiness returns whether agent can use vmcp through instance, its connection to it (the zero instance when it
// has not connected). It only checks the components enabled for the agent, and it is computed anew on every call,
// for the connect path and the readiness API alike. It does not check access: an agent that no profile names has no
// components enabled, and the caller must refuse it.
//
// When more than one profile names every agent, an agent without an explicit profile would take all its setup from
// the ambiguous fallback, so it is refused with one ReadinessSelectionAmbiguous reason for the whole vMCP. An agent
// with an explicit profile is checked component by component, and only the components that need the fallback are
// ambiguous (ComponentReadinessCheck). Tool permissions are unaffected either way: they are the union of every
// profile that names the agent.
func (c AgentReadinessChecks) AgentReadiness(ctx context.Context, vmcp v1.VMCP, instance v1.VMCPInstance, agent kuser.Info) (Readiness, error) {
	if !principal.IsAgent(agent) {
		return Readiness{}, fmt.Errorf("agent readiness is only for agents")
	}
	if vmcp.Spec.UserID != "" {
		return Readiness{}, fmt.Errorf("VMCP %q is personal, and agents only use shared VMCPs", vmcp.Name)
	}

	selection := SelectAgentProfiles(agent, vmcp.Spec.Manifest)
	var readiness Readiness
	if selection.Explicit != nil {
		readiness.ExplicitProfileID = selection.Explicit.ID
	}
	if selection.Wildcard != nil {
		readiness.WildcardProfileID = selection.Wildcard.ID
	}
	if selection.Ambiguous && selection.Explicit == nil {
		// Every configuration value and grant would come from the fallback, which is unknown, so nothing else is
		// checked.
		readiness.Reasons = []ReadinessReason{{Code: ReadinessSelectionAmbiguous}}
		return readiness, nil
	}

	configuration, oauth := c.Configuration, c.OAuth
	if configuration == nil {
		configuration = ProfileConfigurationCheck{}
	}
	if oauth == nil {
		oauth = UnsupportedAgentOAuth{}
	}
	in := AgentReadinessInput{VMCP: vmcp, Instance: instance, Agent: agent, Selection: selection}
	for _, component := range EnabledComponents(agent, vmcp, ComponentsForInstance(vmcp, instance)) {
		for _, check := range []ComponentReadinessCheck{configuration, oauth} {
			reasons, err := check.ComponentReadiness(ctx, in, component)
			if err != nil {
				return Readiness{}, fmt.Errorf("check component %q for agent readiness: %w", component.ID, err)
			}
			// Checks report the same ambiguous fallback for a component that needs both configuration and a grant.
			for _, reason := range reasons {
				if !slices.Contains(readiness.Reasons, reason) {
					readiness.Reasons = append(readiness.Reasons, reason)
				}
			}
		}
	}
	return readiness, nil
}

func (UnsupportedAgentOAuth) ComponentReadiness(_ context.Context, in AgentReadinessInput, component types.VMCPComponent) ([]ReadinessReason, error) {
	if ComponentOAuthCredentialReference(component) == "" {
		return nil, nil
	}
	if in.Selection.Ambiguous {
		// The explicit profile cannot hold a grant yet, so the grant would come from the ambiguous fallback.
		return []ReadinessReason{AmbiguousFallbackReason(component.ID)}, nil
	}
	return []ReadinessReason{{
		ComponentID: component.ID,
		Code:        ReadinessOAuthRequired,
		ProfileID:   in.Selection.fixProfileID(),
	}}, nil
}
