package authz

import (
	"net/http"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/types"
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
