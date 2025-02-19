package authz

import (
	"net/http"
	"strings"

	"k8s.io/apiserver/pkg/authentication/user"
)

func authorizeUI(req *http.Request, _ user.Info) bool {
	if req.Method != http.MethodGet {
		return false
	}
	if strings.HasPrefix(req.URL.Path, "/api") {
		return false
	}

	parts := strings.Split(req.URL.Path, "/")
	if len(parts) > 2 && parts[2] == "projects" {
		return true
	}

	return false
}
