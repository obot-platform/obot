// Package principaltest helps tests build the user info of principals.
package principaltest

import (
	"maps"

	"github.com/obot-platform/obot/pkg/principal"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// AsUser records the user kind on u, as the authenticators of users do, when u's UID is a user's (a decimal ID), so
// that policy matchers read it as that user. Any other principal, such as an anonymous one or one with an agent's
// UID, is returned unchanged.
func AsUser(u *kuser.DefaultInfo) *kuser.DefaultInfo {
	if p, err := principal.Parse(u.UID); err != nil || p.Kind != principal.KindUser {
		return u
	}
	u.Extra = maps.Clone(u.Extra)
	if u.Extra == nil {
		u.Extra = map[string][]string{}
	}
	principal.RecordKind(u.Extra, principal.KindUser)
	return u
}
