package vmcp

import (
	"cmp"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// MatchingProfiles uses both Obot and authentication-provider groups.
func MatchingProfiles(u kuser.Info, profiles []types.VMCPProfile) []types.VMCPProfile {
	groups := slices.Clone(u.GetGroups())
	groups = append(groups, u.GetExtra()["obot_groups"]...)
	groups = append(groups, u.GetExtra()["auth_provider_groups"]...)
	var matching []types.VMCPProfile
	for _, profile := range profiles {
		for _, subject := range profile.Subjects {
			if (subject.Type == types.SubjectTypeSelector && subject.ID == "*") ||
				(subject.Type == types.SubjectTypeUser && subject.ID == u.GetUID()) ||
				(subject.Type == types.SubjectTypeGroup && slices.Contains(groups, subject.ID)) {
				matching = append(matching, profile)
				break
			}
		}
	}
	return matching
}

// AllowedTools returns component-scoped original names. Nil means unrestricted, while an
// empty non-nil slice grants no tools. Instance selections can only narrow grants.
// A name of "*" grants all tools enabled on that component.
func AllowedTools(u kuser.Info, profiles []types.VMCPProfile, selection types.VMCPToolSet) []types.VMCPToolReference {
	var (
		tools []types.VMCPToolReference
		all   bool
	)
	for _, profile := range MatchingProfiles(u, profiles) {
		all = all || profile.AllowAllTools
		tools = append(tools, profile.AllowedTools.References()...)
	}
	if !all && len(tools) == 0 {
		return []types.VMCPToolReference{}
	}
	if selection != nil {
		selected := []types.VMCPToolReference{}
		for _, tool := range selection.References() {
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
