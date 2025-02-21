package authz

import (
	"context"
	"net/http"

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

func (a *Authorizer) checkAssistant(req *http.Request, resources *Resources, user user.Info) (bool, error) {
	if resources.AssistantID == "" {
		return true, nil
	}

	var (
		agentID      = resources.AssistantID
		validUserIDs = getValidUserIDs(user)
		agent        v1.Agent
	)

	if !system.IsAgentID(agentID) {
		if err := alias.Get(req.Context(), a.storage, &agent, "", agentID); err != nil {
			return false, err
		}
		agentID = agent.Name
	}

	if !a.assistantIsAuthorized(req.Context(), agentID, validUserIDs) {
		return false, nil
	}

	resources.Authorizated.Assistant = &agent
	return true, nil
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
