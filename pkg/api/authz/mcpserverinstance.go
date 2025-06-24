package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkMCPServerInstance(req *http.Request, resources *Resources, u user.Info) (bool, error) {
	if resources.MCPServerInstanceID == "" {
		return true, nil
	}

	var mcpServerInstance v1.MCPServerInstance
	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPServerInstanceID), &mcpServerInstance); err != nil {
		return false, err
	}

	return mcpServerInstance.Spec.UserID == u.GetUID(), nil
}
