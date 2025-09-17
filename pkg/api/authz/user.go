package authz

import (
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkUser(user user.Info, userID string) bool {
	return userID == "" ||
		userID == user.GetUID() ||
		slices.Contains(user.GetGroups(), types.GroupAdmin)
}
