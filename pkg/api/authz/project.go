package authz

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkProject(req *http.Request, resources *Resources, user user.Info) (bool, error) {
	if resources.ProjectID == "" {
		return true, nil
	}

	var (
		agentID         string
		validUserIDs    = getValidUserIDs(user)
		thread          v1.Thread
		projectThreadID = strings.Replace(resources.ProjectID, system.ProjectPrefix, system.ThreadPrefix, 1)
	)

	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, projectThreadID), &thread); err != nil {
		return false, err
	}

	if resources.Authorizated.Assistant != nil {
		agentID = resources.Authorizated.Assistant.Name
	}

	if !thread.Spec.Project {
		return false, nil
	}

	if !a.projectIsAuthorized(req.Context(), agentID, &thread, validUserIDs) {
		return false, nil
	}

	resources.Authorizated.Project = &thread
	return true, nil
}

func (a *Authorizer) projectIsAuthorized(ctx context.Context, agentID string, thread *v1.Thread, validUserIDs []string) bool {
	if agentID != "" {
		// If agent is available, make sure it's related
		if thread.Spec.AgentName != agentID {
			return false
		}
	}

	if slices.Contains(validUserIDs, thread.Spec.UserID) {
		return true
	}

	for _, userID := range validUserIDs {
		var access v1.ThreadAuthorizationList
		err := a.storage.List(ctx, &access, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
			"spec.userID":   userID,
			"spec.threadID": thread.Name,
			"spec.accepted": "true",
		})
		if err == nil && len(access.Items) == 1 {
			return true
		}
	}
	return false
}
