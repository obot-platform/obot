package authz

import (
	"slices"

	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkUser(user user.Info, userID string) bool {
	return userID == "" ||
		userID == user.GetUID() ||
		slices.Contains(user.GetGroups(), AdminGroup)
}
