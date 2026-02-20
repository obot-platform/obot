package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkProjectV2(req *http.Request, resources *Resources, u user.Info) (bool, error) {
	if resources.ProjectV2ID == "" {
		return true, nil
	}

	var projectV2 v1.ProjectV2
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.ProjectV2ID), &projectV2); err != nil {
		return false, err
	}

	// If the user owns the project, then authorization is granted.
	if projectV2.Spec.UserID == u.GetUID() {
		resources.Authorizated.ProjectV2 = &projectV2
		return true, nil
	}

	return false, nil
}
