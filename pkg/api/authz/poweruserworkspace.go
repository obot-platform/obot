package authz

import (
	"net/http"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkPowerUserWorkspace(req *http.Request, resources *Resources, user user.Info) (bool, error) {
	if resources.WorkspaceID == "" {
		return true, nil
	}

	var workspace v1.PowerUserWorkspace
	if err := a.get(req.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      resources.WorkspaceID,
	}, &workspace); err != nil {
		return false, err
	}

	if workspace.Spec.UserID == user.GetUID() {
		resources.Authorizated.PowerUserWorkspace = &workspace
		return true, nil
	}

	return false, nil
}
