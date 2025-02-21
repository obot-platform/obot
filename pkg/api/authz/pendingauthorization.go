package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkPendingAuthorization(req *http.Request, resources *Resources, user user.Info) (bool, error) {
	if resources.PendingAuthorizationID == "" {
		return true, nil
	}

	var (
		threadAuth v1.ThreadAuthorization
	)

	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, resources.PendingAuthorizationID), &threadAuth); err != nil {
		return false, err
	}

	for _, uid := range getValidUserIDs(user) {
		if threadAuth.Spec.UserID == uid {
			resources.Authorizated.PendingAuthorization = &threadAuth
			return true, nil
		}
	}

	return true, nil
}
