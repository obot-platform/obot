package principal

import (
	types "github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// UserStatusExtra carries the lifecycle status of the user a principal acts for, as its authenticator read it
	// from the gateway database: the user themselves, or a hosted agent's owner. Code must use RecordUserStatus and
	// UserStatus instead of this key.
	UserStatusExtra = "obot_user_status"
)

// RecordUserStatus records the lifecycle status of the user a principal acts for. It always replaces any value
// already in extra, so that no auth provider can supply one.
func RecordUserStatus(extra map[string][]string, status types.UserStatus) {
	extra[UserStatusExtra] = []string{string(status)}
}

// UserStatus returns the lifecycle status recorded on a principal, and whether its authenticator recorded one.
func UserStatus(user kuser.Info) (types.UserStatus, bool) {
	if user == nil {
		return "", false
	}
	status := user.GetExtra()[UserStatusExtra]
	if len(status) != 1 || status[0] == "" {
		return "", false
	}
	return types.UserStatus(status[0]), true
}
