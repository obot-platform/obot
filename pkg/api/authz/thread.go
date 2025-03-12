package authz

import (
	"net/http"

	"github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkThread(req *http.Request, resources *Resources, user user.Info) (bool, error) {
	if resources.ThreadID == "" {
		return true, nil
	}

	var (
		thread v1.Thread
	)

	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, resources.ThreadID), &thread); err != nil {
		return false, err
	}

	if thread.Spec.Project {
		return false, nil
	}

	if resources.Authorizated.Project == nil {
		threadID := types.FirstSet(user.GetExtra()["obot:threadID"]...)
		agentID := types.FirstSet(user.GetExtra()["obot:agentID"]...)
		if threadID == "" || agentID == "" {
			return false, nil
		}

		return threadID == thread.Name && thread.Spec.AgentName == agentID, nil
	}

	if resources.Authorizated.Project.Name != thread.Spec.ParentThreadName {
		return false, nil
	}

	resources.Authorizated.Thread = &thread
	return true, nil
}
