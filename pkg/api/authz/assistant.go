package authz

import (
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/alias"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) authorizeAssistant(req *http.Request, user user.Info) bool {
	if !strings.HasPrefix(req.URL.Path, "/api/assistants/") {
		return false
	}
	paths := strings.Split(req.URL.Path, "/")
	if paths[3] == "" {
		return false
	}

	var (
		agentID = paths[3]
		keys    = make([]string, 0, 3)
	)
	keys = append(keys, "*", user.GetUID())
	if attr := user.GetExtra()["email"]; len(attr) > 0 {
		keys = append(keys, attr...)
	}

	if !system.IsAgentID(agentID) {
		var agent v1.Agent
		if err := alias.Get(req.Context(), a.storage, &agent, "", agentID); err != nil {
			return false
		}
		agentID = agent.Name
	}

	for _, key := range keys {
		var access v1.AgentAuthorizationList
		err := a.storage.List(req.Context(), &access, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
			"spec.userID":  key,
			"spec.agentID": agentID,
		})
		if err == nil && len(access.Items) == 1 {
			return true
		}
	}

	return false
}
