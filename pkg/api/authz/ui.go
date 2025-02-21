package authz

import (
	"net/http"

	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkUI(req *http.Request, _ *Resources, _ user.Info) (bool, error) {
	var ui = req.PathValue("ui")
	if ui == "" {
		return true, nil
	}
	// Ensure the URL does not start with /api
	return ui != "api", nil
}
