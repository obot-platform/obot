package authz

import (
	"net/http"
)

var uiResources = []string{
	"GET /{assistant}",
	"GET /{assistant}/p/{project}",
	"GET /o/{project}",
	"GET /agent/images/{image}",
	"GET /user/images/{image}",
}

func (a *Authorizer) checkUI(req *http.Request) bool {
	vars, match := a.uiResources.Match(req)
	if !match {
		return false
	}
	if vars("assistant") == "api" {
		return false
	}
	// Matches and is not API
	return true
}
