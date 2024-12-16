package authn

import (
	"net/http"

	"github.com/acorn-io/acorn/pkg/api/authz"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type NoAuth struct {
}

func (n NoAuth) AuthenticateRequest(*http.Request) (*authenticator.Response, bool, error) {
	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   "nobody",
			Groups: []string{authz.AdminGroup, authz.AuthenticatedGroup},
		},
	}, true, nil
}
