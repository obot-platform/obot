package authz

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/name"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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
	if !a.validateWorkspaceRoleAccess(req.URL.Path, isPowerUserPlus, user.GetUID()) {
		return false, nil
	}

	var workspace v1.PowerUserWorkspace
	if err := a.cache.Get(req.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      resources.WorkspaceID,
	}, &workspace); err != nil {
		if err := a.uncached.Get(req.Context(), kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      resources.WorkspaceID,
		}, &workspace); err != nil {
			return false, err
		}
	}

	return workspace.Spec.UserID == user.GetUID(), nil
}

func (a *Authorizer) validateWorkspaceRoleAccess(path string, isPowerUserPlus bool, userID string) bool {
	// PowerUser and PowerUserPlus can access workspace info
	if strings.HasSuffix(path, fmt.Sprintf("/workspaces/%s", name.SafeConcatName(system.PowerUserWorkspacePrefix, userID))) ||
		strings.Contains(path, "/workspaces/") && strings.Contains(path, "/entries") {
		// Both PowerUser and PowerUserPlus can access catalog entries
		return true
	}

	// Only PowerUserPlus can access MCP servers and access control rules
	if strings.Contains(path, "/workspaces/") &&
		(strings.Contains(path, "/servers") || strings.Contains(path, "/access-control-rules")) {
		return isPowerUserPlus
	}

	return false
}
