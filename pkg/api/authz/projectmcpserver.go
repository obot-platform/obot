package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkProjectMCPServer(req *http.Request, resources *Resources, u user.Info) (bool, error) {
	if resources.ProjectMCPServerID == "" {
		return true, nil
	}

	var projectMCPServer v1.ProjectMCPServer
	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, resources.ProjectMCPServerID), &projectMCPServer); err != nil {
		return false, err
	}

	return projectMCPServer.Spec.UserID == u.GetUID(), nil
}
