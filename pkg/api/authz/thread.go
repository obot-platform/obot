package authz

import (
	"net/http"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func authorizeThread(req *http.Request, user user.Info) bool {
	thread := types.FirstSet(user.GetExtra()["otto:threadID"]...)
	agent := types.FirstSet(user.GetExtra()["otto:agentID"]...)
	if thread == "" || agent == "" {
		return false
	}
	if req.Method == "GET" && strings.HasPrefix(req.URL.Path, "/api/threads/"+thread+"/") {
		return true
	}
	if req.Method == "POST" && req.URL.Path == "/api/invoke/"+agent+"/"+"threads/"+thread {
		return true
	}

	return false
}
