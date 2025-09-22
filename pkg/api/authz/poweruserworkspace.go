package authz

import (
	"net/http"
	"regexp"
	"slices"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkPowerUserWorkspace(req *http.Request, resources *Resources, user user.Info) (bool, error) {
	if resources.WorkspaceID == "" {
		return true, nil
	}

	isPowerUser := slices.Contains(user.GetGroups(), PowerUserGroup)
	isPowerUserPlus := slices.Contains(user.GetGroups(), PowerUserPlusGroup)

	if !isPowerUser && !isPowerUserPlus {
		return false, nil
	}

	// Validate role-based access to workspace endpoints
	if !a.validateWorkspaceRoleAccess(req.URL.Path, isPowerUserPlus) {
		return false, nil
	}

	var workspace v1.PowerUserWorkspace
	if err := a.cache.Get(req.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      resources.WorkspaceID,
	}, &workspace); err != nil {
		if errors.IsNotFound(err) {
			if err := a.uncached.Get(req.Context(), kclient.ObjectKey{
				Namespace: system.DefaultNamespace,
				Name:      resources.WorkspaceID,
			}, &workspace); err != nil {
				return false, err
			}
		} else {
			return false, err
		}
	}

	if workspace.Spec.UserID == user.GetUID() {
		resources.Authorizated.PowerUserWorkspace = &workspace
		return true, nil
	}

	return false, nil
}

// Workspace access patterns that require PowerUserPlus privileges, and not PowerUser
var powerUserPlusRequiredPatterns = []*regexp.Regexp{
	regexp.MustCompile(`/workspaces/[^/]+/servers`),              // MCP servers management
	regexp.MustCompile(`/workspaces/[^/]+/access-control-rules`), // Access control rules management
}

func (a *Authorizer) validateWorkspaceRoleAccess(path string, isPowerUserPlus bool) bool {
	// Check patterns that require PowerUserPlus
	for _, pattern := range powerUserPlusRequiredPatterns {
		if pattern.MatchString(path) {
			return isPowerUserPlus
		}
	}

	return true
}
