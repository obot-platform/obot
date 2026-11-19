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
	// Revision is the profile configuration revision that stores the value.
	Revision string `json:"-"`
}

// ComponentConfiguration is how an agent's profile supplies the configuration of one component that a profile can set
// (ProfileConfigurableFields). Everything else the component needs is the vMCP's and its catalog entry's, as for
// everyone.
type ComponentConfiguration struct {
	ComponentID string
	// Sources are the keys the profile supplies, in the order of the component's configuration.
	Sources []ConfigurationSource
	// Missing are the required keys the profile does not supply, sorted.
	Missing []string
}

// AgentConfiguration is how the agent's profile on a vMCP supplies the configuration of the components enabled for the
// agent.
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
// its connection (the zero instance when it has not connected), from the keys that the current revision of its profile
// (AgentProfile) sets (v1.VMCPStatus.ProfileConfigurationRevisions). See ResolveComponentConfiguration for the rules.
// It reads no values, so it is cheap enough to compute on every connect.
func ResolveAgentConfiguration(vmcp v1.VMCP, instance v1.VMCPInstance, agent kuser.Info) AgentConfiguration {
	profile := AgentProfile(agent, vmcp.Spec.Manifest)
	var result AgentConfiguration
	var sources []ConfigurationSource
	for _, component := range EnabledComponents(agent, vmcp, ComponentsForInstance(vmcp, instance)) {
		resolved := ResolveComponentConfiguration(component, profile, vmcp.Status.ProfileConfigurationRevisions)
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

// ResolveComponentConfiguration resolves how an agent's profile supplies the configuration of component, from the keys
// that each profile's current revision sets (revisions). Each key the profile sets is supplied by it, and a required
// key it does not set is missing. No other profile supplies anything, since no other profile names the agent. A
// profile that does not grant the component supplies nothing for it (profileForComponent). Fixed, catalog-static and
// Secret-bound values are never a profile's, so they stay as the vMCP and its catalog entry set them.
func ResolveComponentConfiguration(component types.VMCPComponent, profile *types.VMCPProfile, revisions map[string]v1.VMCPProfileConfigurationRevision) ComponentConfiguration {
	profile = profileForComponent(profile, component.ID)
	result := ComponentConfiguration{ComponentID: component.ID}
	var revision v1.VMCPProfileConfigurationRevision
	if profile != nil {
		revision = revisions[profile.ID]
	}
	for _, field := range ProfileConfigurableFields(component) {
		switch {
		case revision.Name != "" && slices.Contains(revision.Keys, ConfigurationKey(component.ID, field.Key)):
			result.Sources = append(result.Sources, ConfigurationSource{
				ComponentID: component.ID,
				Field:       field.Key,
				ProfileID:   profile.ID,
				Revision:    revision.Name,
			})
		case field.Required:
			result.Missing = append(result.Missing, field.Key)
		}
	}
	slices.Sort(result.Missing)
	return result
}

// configurationReasons returns why an agent's configuration of a component, as its profile resolves it (resolved,
// ResolveComponentConfiguration), keeps it from using the component, where profileID is the agent's profile. A
// required value that a user would enter on their own connection, and that the agent's profile does not supply, is a
// types.AgentReadinessConfigurationRequired reason, fixed in that profile.
//
// Resolution reads the keys that the vMCP's status publishes for each profile's current revision, never the values, so
// it costs nothing on the connect path, and the readiness API computes the same answer. The connect path checks it only
// after the agent's instance holds the values of those same revisions.
func configurationReasons(resolved ComponentConfiguration, profileID string) []ReadinessReason {
	reasons := make([]ReadinessReason, 0, len(resolved.Missing))
	for _, field := range resolved.Missing {
		reasons = append(reasons, ReadinessReason{
			ComponentID: resolved.ComponentID,
			Code:        types.AgentReadinessConfigurationRequired,
			Field:       field,
			ProfileID:   profileID,
		})
	}
	return reasons
}
