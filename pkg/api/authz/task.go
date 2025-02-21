package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkTask(req *http.Request, resources *Resources, _ user.Info) (bool, error) {
	if resources.TaskID == "" {
		return true, nil
	}

	if resources.Authorizated.Project == nil {
		return false, nil
	}

	var (
		workflow v1.Workflow
	)

	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, resources.TaskID), &workflow); err != nil {
		return false, err
	}

	if resources.Authorizated.Project.Name != workflow.Spec.ThreadName {
		return false, nil
	}

	resources.Authorizated.Task = &workflow
	return true, nil
}
