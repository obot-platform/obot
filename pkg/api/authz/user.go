package authz

import (
	"github.com/obot-platform/obot/apiclient/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

type User struct {
	kuser.Info

	IsAdmin        bool
	IsAuditor      bool
	IsOwner        bool
	CanImpersonate bool
}

func newUser(user kuser.Info) User {
	u := User{
		Info: user,
	}

	for _, group := range user.GetGroups() {
		switch group {
		case types.GroupAdmin:
			u.IsAdmin = true
		case types.GroupAuditor:
			u.IsAuditor = true
		case types.GroupOwner:
			u.IsOwner = true
		case types.GroupUserImpersonation:
			u.CanImpersonate = true
		}
	}

	return u
}

func (a *Authorizer) checkUser(user User, userID string) bool {
	return userID == "" ||
		userID == user.GetUID() ||
		user.IsAdmin
}
