package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkMCPServer(req *http.Request, resources *Resources, _ user.Info) (bool, error) {
	if resources.MCPServerID == "" {
		return true, nil
	}

	var (
		mcpServer v1.MCPServer
	)

	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPServerID), &mcpServer); err != nil {
		return false, err
	}

	if resources.Authorizated.Project == nil && resources.Authorizated.Thread != nil &&
		resources.Authorizated.Thread.Spec.ParentThreadName != "" &&
		resources.Authorizated.Thread.Spec.ParentThreadName == mcpServer.Spec.ThreadName {
		resources.Authorizated.MCPServer = &mcpServer
		return true, nil
	}

	if resources.Authorizated.Project == nil {
		return false, nil
	}

	if resources.Authorizated.Project.Name == mcpServer.Spec.ThreadName ||
		resources.Authorizated.Project.Spec.ParentThreadName == mcpServer.Spec.ThreadName {
		resources.Authorizated.MCPServer = &mcpServer
		return true, nil
	}

	return false, nil
}
