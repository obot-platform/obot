package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkRun(req *http.Request, resources *Resources, _ user.Info) (bool, error) {
	if resources.RunID == "" {
		return true, nil
	}

	if resources.Authorizated.Task == nil {
		return false, nil
	}

	var (
		wfe v1.WorkflowExecution
	)

	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, resources.RunID), &wfe); err != nil {
		return false, err
	}

	if resources.Authorizated.Task.Name != wfe.Spec.WorkflowName {
		return false, nil
	}

	resources.Authorizated.Run = &wfe
	return true, nil
}
