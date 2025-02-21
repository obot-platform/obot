package authz

import (
	"net/http"
)

var uiResources = []string{
	"GET /{$}",
	"GET /admin/",
	"GET /agent/images/",
	"GET /_app/",
	"GET /{assistant}",
	"GET /o/",
	"GET /s/",
	"GET /user/images/",
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
