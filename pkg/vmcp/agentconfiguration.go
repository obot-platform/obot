package vmcp

import (
	"cmp"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// ConfigurationSource is the profile that supplies an agent's value for one configuration key. It names the key, never
// the value.
type ConfigurationSource struct {
	ComponentID string `json:"componentID"`
	Field       string `json:"field"`
	ProfileID   string `json:"profileID"`
	// Fallback reports that the value comes from the profile that names every agent, which the agent falls back to,
	// rather than from the profile that names the agent itself.
	Fallback bool `json:"fallback,omitempty"`
	// Revision is the profile configuration revision that stores the value.
	Revision string `json:"-"`
}

// ComponentConfiguration is how an agent's profiles supply the configuration of one component that a profile can set
// (ProfileConfigurableFields). Everything else the component needs is the vMCP's and its catalog entry's, as for
// everyone.
type ComponentConfiguration struct {
	ComponentID string
	// Sources are the keys a profile supplies, in the order of the component's configuration.
	Sources []ConfigurationSource
	// Missing are the required keys no profile supplies, sorted.
	Missing []string
	// AmbiguousFallback reports that the agent's explicit profile does not supply every required key, and that the
	// profile it would fall back to for the rest is ambiguous (AgentSelection.Ambiguous). Missing is then empty: the
	// keys are not missing from any known profile.
	AmbiguousFallback bool
}

// AgentConfiguration is how the profiles of a vMCP supply the configuration of the components enabled for an agent.
type AgentConfiguration struct {
	Components []ComponentConfiguration
	// Revision identifies the resolution: which revision of which profile supplies each key. The vMCP instance
	// controller records it on the agent's instance once it has copied those values there
	// (v1.VMCPInstanceStatus.AgentConfigurationRevision), so an instance whose revision differs from the one resolved
	// from the vMCP's current state has not caught up. It is never empty.
	Revision string
	// Revisions are the profile configuration revisions, by profile ID, that supply any value. It is nil when none do.
	Revisions map[string]string
}

// ResolveAgentConfiguration resolves the configuration of the components of vmcp enabled for agent, through instance,
// its connection (the zero instance when it has not connected), from the keys that its profiles' current revisions set
// (v1.VMCPStatus.ProfileConfigurationRevisions). See ResolveComponentConfiguration for the rules. It reads no values,
// so it is cheap enough to compute on every connect.
func ResolveAgentConfiguration(vmcp v1.VMCP, instance v1.VMCPInstance, agent kuser.Info) AgentConfiguration {
	selection := SelectAgentProfiles(agent, vmcp.Spec.Manifest)
	var result AgentConfiguration
	var sources []ConfigurationSource
	for _, component := range EnabledComponents(agent, vmcp, ComponentsForInstance(vmcp, instance)) {
		resolved := ResolveComponentConfiguration(component, selection, vmcp.Status.ProfileConfigurationRevisions)
		result.Components = append(result.Components, resolved)
		sources = append(sources, resolved.Sources...)
	}
	for _, source := range sources {
		if result.Revisions == nil {
			result.Revisions = map[string]string{}
		}
		result.Revisions[source.ProfileID] = source.Revision
	}
	// The values copied to the instance depend only on which revision each key is copied from, so that is what the
	// revision identifies, in an order that does not depend on the manifest's.
	slices.SortFunc(sources, func(a, b ConfigurationSource) int {
		return cmp.Or(cmp.Compare(a.ComponentID, b.ComponentID), cmp.Compare(a.Field, b.Field))
	})
	identity := make([][3]string, 0, len(sources))
	for _, source := range sources {
		identity = append(identity, [3]string{ConfigurationKey(source.ComponentID, source.Field), source.ProfileID, source.Revision})
	}
	result.Revision = utils.Digest([]any{"agent-profile-configuration/v1", identity})
	return result
}

// FallbackProfileIDs returns the profiles that supply a value to the agent as the profile it falls back to.
func (c AgentConfiguration) FallbackProfileIDs() []string {
	var ids []string
	for _, component := range c.Components {
		for _, source := range component.Sources {
			if source.Fallback && !slices.Contains(ids, source.ProfileID) {
				ids = append(ids, source.ProfileID)
			}
		}
	}
	return ids
}

// ResolveComponentConfiguration resolves how an agent's profiles supply the configuration of component, from the keys
// that each profile's current revision sets (revisions). The explicit profile, which names the agent by ID, comes
// first. A required key it does not set falls back to the wildcard profile, which names every agent. An optional key
// never falls back: with an explicit profile, an optional key it does not set stays unset, and with none, the
// wildcard profile's configuration is used as it is. Removing an agent's explicit profile makes it fall back for
// every key; it does not revoke the wildcard's.
//
// When the wildcard profile is ambiguous (AgentSelection.Ambiguous), nothing falls back, and a component that would
// need the fallback reports AmbiguousFallback. Fixed, catalog-static and Secret-bound values are never a profile's, so
// they stay as the vMCP and its catalog entry set them.
func ResolveComponentConfiguration(component types.VMCPComponent, selection AgentSelection, revisions map[string]v1.VMCPProfileConfigurationRevision) ComponentConfiguration {
	result := ComponentConfiguration{ComponentID: component.ID}
	supply := func(profile *types.VMCPProfile, field types.MCPConfig, fallback bool) bool {
		if profile == nil {
			return false
		}
		revision := revisions[profile.ID]
		if revision.Name == "" || !slices.Contains(revision.Keys, ConfigurationKey(component.ID, field.Key)) {
			return false
		}
		result.Sources = append(result.Sources, ConfigurationSource{
			ComponentID: component.ID,
			Field:       field.Key,
			ProfileID:   profile.ID,
			Fallback:    fallback,
			Revision:    revision.Name,
		})
		return true
	}
	for _, field := range ProfileConfigurableFields(component) {
		switch {
		case supply(selection.Explicit, field, false):
		case selection.Explicit != nil && !field.Required:
			// Optional values never fall back.
		case selection.Ambiguous:
			result.AmbiguousFallback = result.AmbiguousFallback || field.Required
		case supply(selection.Wildcard, field, true):
		case field.Required:
			result.Missing = append(result.Missing, field.Key)
		}
	}
	if result.AmbiguousFallback {
		result.Missing = nil
	}
	slices.Sort(result.Missing)
	return result
}

// configurationReasons returns why an agent's configuration of a component, as its profiles resolve it (resolved,
// ResolveComponentConfiguration), keeps it from using the component, where selection is how the agent's profiles apply
// to it. A required value that a user would enter on their own connection, and that neither the agent's explicit
// profile nor, falling back, the wildcard profile supplies, is a types.AgentReadinessConfigurationRequired reason,
// fixed in the explicit profile if the agent has one and otherwise in the wildcard. A component that needs the fallback
// while it is ambiguous is AmbiguousFallbackReason.
//
// Resolution reads the keys that the vMCP's status publishes for each profile's current revision, never the values, so
// it costs nothing on the connect path, and the readiness API computes the same answer. The connect path checks it only
// after the agent's instance holds the values of those same revisions.
func configurationReasons(resolved ComponentConfiguration, selection AgentSelection) []ReadinessReason {
	if resolved.AmbiguousFallback {
		return []ReadinessReason{AmbiguousFallbackReason(resolved.ComponentID)}
	}
	reasons := make([]ReadinessReason, 0, len(resolved.Missing))
	for _, field := range resolved.Missing {
		reasons = append(reasons, ReadinessReason{
			ComponentID: resolved.ComponentID,
			Code:        types.AgentReadinessConfigurationRequired,
			Field:       field,
			ProfileID:   selection.fixProfileID(),
		})
	}
	return reasons
}
