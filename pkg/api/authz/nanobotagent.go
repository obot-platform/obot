package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (a *Authorizer) checkNanobotAgent(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.NanobotAgentID == "" {
		return true, nil
	}

	var agent v1.NanobotAgent
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.NanobotAgentID), &agent); err != nil {
		return false, err
	}

	// If the user owns the workflow, then authorization is granted.
	if agent.Spec.UserID == u.GetUID() {
		resources.Authorizated.NanobotAgent = &agent
		return true, nil
	}

	// If the workflow belongs to a project and the user owns that project, authorization is granted.
	if resources.Authorizated.Project != nil && resources.Authorizated.Project.Spec.UserID == u.GetUID() && agent.Spec.ProjectID == resources.Authorizated.Project.Name {
		resources.Authorizated.NanobotAgent = &agent
		return true, nil
	}

	// If the user has impersonation + admin privileges, allow access to any agent.
	if u.CanImpersonate && u.IsAdmin {
		resources.Authorizated.NanobotAgent = &agent
		return true, nil
	}

	return false, nil
}
