package vmcp

import (
	"cmp"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// PruneRemovedComponentProfiles removes grants only for components that were
// actually removed, leaving unrelated invalid references for validation.
func PruneRemovedComponentProfiles(previous []types.VMCPComponent, manifest *types.VMCPManifest) {
	for _, component := range previous {
		if slices.ContainsFunc(manifest.Components, func(current types.VMCPComponent) bool { return current.ID == component.ID }) {
			continue
		}
		for i := range manifest.Profiles {
			delete(manifest.Profiles[i].Permissions.AllowedComponents, component.ID)
		}
	}
}

// MatchingProfiles returns the profiles whose subjects name u, in order (subjects.Evaluate). Users match their user
// ID, subjects:*, and their auth provider and Obot groups; agents match only their agent ID, so an agent matches at
// most one profile (AgentProfile). A caller that is not a well-formed user or agent principal matches no profile.
func MatchingProfiles(u kuser.Info, profiles []types.VMCPProfile) []types.VMCPProfile {
	caller, ok := subjects.CallerFromUserInfo(u)
	if !ok {
		return nil
	}
	var matching []types.VMCPProfile
	for _, profile := range profiles {
		if caller.Matches(profile.Subjects) {
			matching = append(matching, profile)
		}
	}
	return matching
}

// AllowedTools returns component-scoped original names. Nil means unrestricted, while an
// empty non-nil slice grants no tools. Instance selections can only narrow grants.
// A name of "*" grants all tools enabled on that component.
func AllowedTools(u kuser.Info, profiles []types.VMCPProfile, selection map[string]types.VMCPComponentSet) []types.VMCPToolReference {
	var (
		tools []types.VMCPToolReference
		all   bool
	)
	for _, profile := range MatchingProfiles(u, profiles) {
		all = all || profile.Permissions.AllowAllComponents
		tools = append(tools, types.ComponentToolReferences(profile.Permissions.AllowedComponents)...)
	}
	if !all && len(tools) == 0 {
		return []types.VMCPToolReference{}
	}
	if selection != nil {
		selected := []types.VMCPToolReference{}
		for _, tool := range types.ComponentToolReferences(selection) {
			if all || ToolGranted(tools, tool) {
				selected = append(selected, tool)
			} else if tool.Name == "*" {
				// A revoked component wildcard contracts to its remaining grants.
				for _, granted := range tools {
					if granted.ComponentID == tool.ComponentID {
						selected = append(selected, granted)
					}
				}
			}
		}
		tools = selected
	} else if all {
		return nil
	}
	slices.SortFunc(tools, func(a, b types.VMCPToolReference) int {
		if n := cmp.Compare(a.ComponentID, b.ComponentID); n != 0 {
			return n
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return slices.Compact(tools)
}

// ToolGranted checks an exact name or wildcard within the same component.
// A nil grant is unrestricted; an empty grant permits nothing.
func ToolGranted(grant []types.VMCPToolReference, tool types.VMCPToolReference) bool {
	return grant == nil || slices.Contains(grant, tool) ||
		slices.Contains(grant, types.VMCPToolReference{ComponentID: tool.ComponentID, Name: "*"})
}

// InstanceGrant returns the tools an instance's user may call. The user is only
// consulted for shared VMCPs; a personal VMCP grants only its owner's selection.
// Nil means unrestricted, while an empty non-nil slice grants no tools.
func InstanceGrant(u kuser.Info, vmcp v1.VMCP, instance v1.VMCPInstance) []types.VMCPToolReference {
	switch vmcp.Spec.UserID {
	case "":
		return AllowedTools(u, vmcp.Spec.Manifest.Profiles, instance.Spec.Manifest.ComponentSet)
	case instance.Spec.UserID:
		return types.ComponentToolReferences(instance.Spec.Manifest.ComponentSet)
	}
	return []types.VMCPToolReference{}
}

// ProfileGrantsComponent reports whether profile grants the component with componentID: it allows all components, or
// has an entry for the component in AllowedComponents, even one that grants no tools. A profile's setup for agents
// covers only the components it grants, and only those take their configuration and OAuth grant from it
// (profileForComponent).
func ProfileGrantsComponent(profile types.VMCPProfile, componentID string) bool {
	if profile.Permissions.AllowAllComponents {
		return true
	}
	_, ok := profile.Permissions.AllowedComponents[componentID]
	return ok
}

// EnabledComponents removes the components that no matching profile enables.
// A component is enabled by a profile that grants it (ProfileGrantsComponent).
// Disabled components need no connection, configuration, or OAuth. Profiles do
// not apply to personal VMCPs.
func EnabledComponents(u kuser.Info, vmcp v1.VMCP, components []types.VMCPComponent) []types.VMCPComponent {
	if vmcp.Spec.UserID != "" {
		return components
	}
	matching := MatchingProfiles(u, vmcp.Spec.Manifest.Profiles)
	return slices.DeleteFunc(components, func(component types.VMCPComponent) bool {
		return !slices.ContainsFunc(matching, func(profile types.VMCPProfile) bool { return ProfileGrantsComponent(profile, component.ID) })
	})
}

// GrantedToolOverrides narrows a component's tool overrides to a grant, matching
// stable component identities and original tool names. The second result is false
// when the grant leaves the component unrestricted. Without overrides, every
// discovered tool is available, so the granted names become enabled overrides.
func GrantedToolOverrides(componentID string, overrides []types.ToolOverride, grant []types.VMCPToolReference) ([]types.ToolOverride, bool) {
	if grant == nil {
		return overrides, false
	}
	var (
		granted = map[string]struct{}{}
		names   []string
	)
	for _, ref := range grant {
		if ref.ComponentID != componentID {
			continue
		}
		if ref.Name == "*" {
			return overrides, false
		}
		if _, ok := granted[ref.Name]; !ok {
			granted[ref.Name] = struct{}{}
			names = append(names, ref.Name)
		}
	}

	var tools []types.ToolOverride
	if len(overrides) > 0 {
		for _, tool := range overrides {
			if _, ok := granted[tool.Name]; ok && tool.Enabled {
				tools = append(tools, tool)
			}
		}
	} else {
		for _, name := range names {
			tools = append(tools, types.ToolOverride{Name: name, Enabled: true})
		}
	}
	return tools, true
}
