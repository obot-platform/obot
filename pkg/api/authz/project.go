package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (a *Authorizer) checkProject(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.ProjectID == "" {
		return true, nil
	}

	var project v1.Project
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.ProjectID), &project); err != nil {
		return false, err
	}

	// If the user owns the project, then authorization is granted.
	if project.Spec.UserID == u.GetUID() {
		resources.Authorizated.Project = &project
		return true, nil
	}

	// If the user has impersonation + admin privileges, allow access to any project.
	if u.CanImpersonate && u.IsAdmin {
		resources.Authorizated.Project = &project
		return true, nil
	}

	return false, nil
}
