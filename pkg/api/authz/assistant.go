package authz

import (
	"net/http"

	"github.com/obot-platform/obot/pkg/alias"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getValidUserIDs(user user.Info) []string {
	keys := make([]string, 0, 3)
	keys = append(keys, "*", user.GetUID())
	if attr := user.GetExtra()["email"]; len(attr) > 0 {
		keys = append(keys, attr...)
	}
	if attr := user.GetExtra()["obot:userID"]; len(attr) > 0 {
		keys = append(keys, attr...)
	}
	return keys
}

func (a *Authorizer) checkAssistant(req *http.Request, resources *Resources, _ user.Info) (bool, error) {
	if resources.AssistantID == "" {
		return true, nil
	}

	var (
		agentID = resources.AssistantID
		agent   v1.Agent
	)

	if !system.IsAgentID(agentID) {
		if err := alias.Get(req.Context(), a.storage, &agent, "", agentID); err != nil {
			return false, err
		}
	} else {
		if err := a.storage.Get(req.Context(), client.ObjectKey{Name: agentID, Namespace: system.DefaultNamespace}, &agent); err != nil {
			return false, err
		}
	}

	// All users are only allowed to access the default assistant.
	if agent.Spec.Manifest.Default {
		resources.Authorizated.Assistant = &agent
		return true, nil
	}

	return false, nil
}
