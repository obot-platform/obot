package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkTools(req *http.Request, resources *Resources, _ user.Info) (bool, error) {
	if resources.ToolID == "" {
		return true, nil
	}

	if resources.Authorizated.Project == nil {
		return false, nil
	}

	var tool v1.Tool
	if err := a.storage.Get(req.Context(), router.Key(resources.Authorizated.Project.Namespace,
		resources.ToolID), &tool); err != nil {
		return false, err
	}

	if tool.Spec.ThreadName != resources.Authorizated.Project.Name {
		return false, nil
	}

	resources.Authorizated.Tool = &tool
	return true, nil
}
