package principal

import (
	"context"

	types "github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

const (
	// UserStatusExtra carries the lifecycle status of the user a principal acts for, as its authenticator read it
	// from the gateway database. Code must use RecordUserStatus and
	// UserStatus instead of this key.
	UserStatusExtra = "obot_user_status"
	// KindExtra carries the kind of identity a principal acts for, as its authenticator determined it. Code must use
	// RecordKind and FromUserInfo instead of this key.
	KindExtra = "obot_principal_kind"
)

type admittedPrincipalKey struct{}

// RecordUserStatus records the lifecycle status of the user a principal acts for. It always replaces any value
// already in extra, so that no auth provider can supply one.
func RecordUserStatus(extra map[string][]string, status types.UserStatus) {
	extra[UserStatusExtra] = []string{string(status)}
}

// RecordKind records the kind of identity a principal acts for. It always replaces any value already in extra, so
// that no auth provider can supply one.
//
// It also removes any user the principal is recorded to act for (RecordOnBehalfOf). Every authenticator of users and
// agents records a kind, so a caller acts for a user only when its own authenticator records that after the kind, and
// never because an auth provider supplied it.
func RecordKind(extra map[string][]string, kind Kind) {
	extra[KindExtra] = []string{string(kind)}
	delete(extra, OnBehalfOfExtra)
}

// RecordPrincipal records the kind of identity a principal acts for and its lifecycle status, as every authenticator
// of users and agents must (RecordKind and RecordUserStatus).
func RecordPrincipal(extra map[string][]string, kind Kind, status types.UserStatus) {
	RecordKind(extra, kind)
	RecordUserStatus(extra, status)
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
// user was active when the request arrived. A principal acts for its own user. userID is the storage encoding of a
// principal (Principal.String), so this holds for an agent's encoded ID too, and a user and an agent that share a
// numeric ID never match each other.
func AdmittedActiveUser(ctx context.Context, userID string) bool {
	user, _ := ctx.Value(admittedPrincipalKey{}).(kuser.Info)
	if user == nil || userID == "" {
		return false
	}
	if status, recorded := UserStatus(user); !recorded || status != types.UserStatusActive {
		return false
	}
	return user.GetUID() == userID
}
