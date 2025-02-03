package authz

import (
	"net/http"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func authorizeThread(req *http.Request, user user.Info) bool {
	thread := types.FirstSet(user.GetExtra()["obot:threadID"]...)
	agent := types.FirstSet(user.GetExtra()["obot:agentID"]...)
	if thread == "" || agent == "" {
		return false
	}
	if req.Method == "GET" && strings.HasPrefix(req.URL.Path, "/api/threads/"+thread+"/") {
		return true
	}
	if req.Method == "POST" && strings.HasPrefix(req.URL.Path, "/api/threads/"+thread+"/tasks/") {
		return true
	}

	return false
}

func (a *Authorizer) authorizeThreadFileDownload(req *http.Request, user user.Info) bool {
	if req.Method != http.MethodGet {
		return false
	}

	if !strings.HasPrefix(req.URL.Path, "/api/threads/") {
		return false
	}

	parts := strings.Split(req.URL.Path, "/")
	if len(parts) < 6 {
		return false
	}
	if parts[0] != "" ||
		parts[1] != "api" ||
		parts[2] != "threads" ||
		parts[4] != "files" {
		return false
	}

	var (
		id     = parts[3]
		thread v1.Thread
	)
	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, id), &thread); err != nil {
		return false
	}

	if thread.Spec.UserUID == user.GetUID() {
		return true
	}

	if thread.Spec.WorkflowName == "" {
		return false
	}

	var workflow v1.Workflow
	if err := a.storage.Get(req.Context(), router.Key(thread.Namespace, thread.Spec.WorkflowName), &workflow); err != nil {
		return false
	}

	if workflow.Spec.ThreadName == "" {
		return false
	}

	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, workflow.Spec.ThreadName), &thread); err != nil {
		return false
	}

	return thread.Spec.UserUID == user.GetUID()
}
