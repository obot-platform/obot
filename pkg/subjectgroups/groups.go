package subjectgroups

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func Sets(user kuser.Info) (authProvider, obot map[string]struct{}) {
	if user == nil {
		return nil, nil
	}

	extra := user.GetExtra()
	authProvider = make(map[string]struct{}, len(extra["auth_provider_groups"]))
	for _, group := range extra["auth_provider_groups"] {
		authProvider[group] = struct{}{}
	}
	obot = make(map[string]struct{}, len(extra["obot_groups"]))
	for _, group := range extra["obot_groups"] {
		obot[group] = struct{}{}
	}
	return authProvider, obot
}

// IsAdmin reports whether the user is an admin or owner, by their groups or their current Obot role groups.
// Tokens that carry no role groups, such as MCP tokens, are checked by the role groups alone.
func IsAdmin(user kuser.Info) bool {
	if user == nil {
		return false
	}
	for _, groups := range [][]string{user.GetGroups(), user.GetExtra()["obot_groups"]} {
		if slices.Contains(groups, types.GroupAdmin) || slices.Contains(groups, types.GroupOwner) {
			return true
		}
	}
	return false
}
