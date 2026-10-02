package principal

import (
	"context"

	types "github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// UserStatusExtra carries the lifecycle status of the user a principal acts for, as its authenticator read it
	// from the gateway database: the user themselves, or a hosted agent's owner. Code must use RecordUserStatus and
	// UserStatus instead of this key.
	UserStatusExtra = "obot_user_status"
)

type admittedPrincipalKey struct{}

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

// WithAdmittedPrincipal returns ctx carrying the principal of a request that the admission check let through. Work
// done for the request can then trust the lifecycle status recorded on the principal.
func WithAdmittedPrincipal(ctx context.Context, user kuser.Info) context.Context {
	return context.WithValue(ctx, admittedPrincipalKey{}, user)
}

// AdmittedActiveUser reports whether ctx carries an admitted principal that acts for the user with userID, and that
// user was active when the request arrived. A principal acts for its own user, and a hosted agent for its owner.
func AdmittedActiveUser(ctx context.Context, userID string) bool {
	user, _ := ctx.Value(admittedPrincipalKey{}).(kuser.Info)
	if user == nil || userID == "" {
		return false
	}
	if status, recorded := UserStatus(user); !recorded || status != types.UserStatusActive {
		return false
	}
	if IsHostedAgent(user) {
		return ResourceOwnerID(user) == userID
	}
	return user.GetUID() == userID
}
