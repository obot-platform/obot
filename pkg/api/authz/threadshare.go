package authz

import (
	"net/http"
	"slices"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkThreadShare(req *http.Request, resources *Resources, user user.Info) (bool, error) {
	if resources.ThreadShareID == "" {
		return true, nil
	}

	var (
		threadShareList v1.ThreadShareList
	)

	err := a.storage.List(req.Context(), &threadShareList, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
		"spec.publicID": resources.ThreadShareID,
	})
	if err != nil {
		return false, err
	}

	validUserIDs := getValidUserIDs(user)
	for _, threadShare := range threadShareList.Items {
		if threadShare.Spec.UserID == user.GetUID() {
			resources.Authorizated.ThreadShare = &threadShare
			return true, nil
		}

		if threadShare.Spec.Manifest.Public {
			return true, nil
		}

		for _, uid := range validUserIDs {
			if slices.Contains(threadShare.Spec.Manifest.Users, uid) {
				resources.Authorizated.ThreadShare = &threadShare
				return true, nil
			}
		}
	}

	return false, nil
}
