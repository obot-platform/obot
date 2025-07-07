package authz

import (
	"net/http"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkMCPID(req *http.Request, resources *Resources, user user.Info) (bool, error) {
	if resources.MCPID == "" {
		return true, nil
	}

	if strings.HasPrefix(resources.MCPID, system.MCPServerInstancePrefix) {
		var mcpServerInstance v1.MCPServerInstance
		if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPID), &mcpServerInstance); err != nil {
			return false, err
		}

		return mcpServerInstance.Spec.UserID == user.GetUID(), nil
	}

	var mcpServer v1.MCPServer
	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPID), &mcpServer); err != nil {
		return false, err
	}

	// For servers, we only allow it if the user owns the server.
	// Shared servers should be interacted with using MCPServerInstances.
	return mcpServer.Spec.UserID == user.GetUID(), nil
}
