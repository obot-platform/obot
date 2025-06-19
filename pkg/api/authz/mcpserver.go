package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkMCPServer(req *http.Request, resources *Resources, u user.Info) (bool, error) {
	if resources.MCPServerID == "" {
		return true, nil
	}

	var (
		mcpServer v1.MCPServer
	)

	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, resources.MCPServerID), &mcpServer); err != nil {
		return false, err
	}

	// If this MCP server is shared within a catalog that the user has access to,
	// then authorization is granted.
	if mcpServer.Spec.SharedWithinMCPCatalogName != "" {
		var userAuths v1.UserCatalogAuthorizationList
		if err := a.storage.List(req.Context(), &userAuths, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
			"spec.mcpCatalogName": mcpServer.Spec.SharedWithinMCPCatalogName,
		}); err != nil {
			return false, err
		}

		for _, auth := range userAuths.Items {
			if auth.Spec.UserID == u.GetUID() || auth.Spec.UserID == "*" {
				resources.Authorizated.MCPServer = &mcpServer
				return true, nil
			}
		}

		return false, nil
	}

	// Check to see if this MCP server is shared within a project that the user has access to.

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
