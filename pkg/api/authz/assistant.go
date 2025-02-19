package authz

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/alias"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func getValidUserIDs(user user.Info) []string {
	keys := make([]string, 0, 3)
	keys = append(keys, "*", user.GetUID())
	if attr := user.GetExtra()["email"]; len(attr) > 0 {
		keys = append(keys, attr...)
	}
	return keys
}

func (a *Authorizer) assistantIsAuthorized(ctx context.Context, agentID string, validUserIDs []string) bool {
	for _, userID := range validUserIDs {
		var access v1.AgentAuthorizationList
		err := a.storage.List(ctx, &access, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
			"spec.userID":  userID,
			"spec.agentID": agentID,
		})
		if err == nil && len(access.Items) == 1 {
			return true
		}
	}
	return false
}

func (a *Authorizer) threadIsAuthorized(ctx context.Context, agentID, projectID, threadID string, user user.Info) bool {
	var thread v1.Thread
	if err := a.storage.Get(ctx, router.Key(system.DefaultNamespace, threadID), &thread); err != nil {
		return false
	}
	if thread.Spec.AgentName != agentID {
		return false
	}
	if thread.Spec.ParentThreadName != strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1) {
		return false
	}
	if thread.Spec.UserUID != user.GetUID() {
		return false
	}
	return true
}

func (a *Authorizer) projectIsAuthorized(ctx context.Context, agentID, projectID string, validUserIDs []string) bool {
	var (
		thread   v1.Thread
		threadID = strings.Replace(projectID, system.ProjectPrefix, system.ThreadPrefix, 1)
	)
	if err := a.storage.Get(ctx, router.Key(system.DefaultNamespace, threadID), &thread); err != nil {
		return false
	}
	if !thread.Spec.Project {
		return false
	}
	if thread.Spec.AgentName != agentID {
		return false
	}
	if slices.Contains(validUserIDs, thread.Spec.UserUID) {
		return true
	}

	for _, userID := range validUserIDs {
		var access v1.ThreadAuthorizationList
		err := a.storage.List(ctx, &access, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
			"spec.userID":   userID,
			"spec.threadID": threadID,
			"spec.accepted": "true",
		})
		if err == nil && len(access.Items) == 1 {
			return true
		}
	}
	return false
}

func (a *Authorizer) authorizeAssistant(req *http.Request, user user.Info) bool {
	if !strings.HasPrefix(req.URL.Path, "/api/assistants/") {
		return false
	}

	paths := strings.Split(req.URL.Path, "/")
	if paths[3] == "" {
		return false
	}

	// Must be authenticated
	if !slices.Contains(user.GetGroups(), AuthenticatedGroup) {
		return false
	}

	var (
		agentID      = paths[3]
		validUserIDs = getValidUserIDs(user)
	)

	if !system.IsAgentID(agentID) {
		var agent v1.Agent
		if err := alias.Get(req.Context(), a.storage, &agent, "", agentID); err != nil {
			return false
		}
		agentID = agent.Name
	}

	if !a.assistantIsAuthorized(req.Context(), agentID, validUserIDs) {
		return false
	}

	if len(paths) <= 5 || paths[4] != "projects" {
		return true
	}

	// Emails are authorized only here, so reverse the list
	slices.Reverse(validUserIDs)

	var projectID = paths[5]
	if !a.projectIsAuthorized(req.Context(), agentID, projectID, validUserIDs) {
		return false
	}

	if len(paths) <= 7 || paths[6] != "threads" {
		return true
	}

	var threadID = paths[7]
	return a.threadIsAuthorized(req.Context(), agentID, projectID, threadID, user)
}
