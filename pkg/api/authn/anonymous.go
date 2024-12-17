package authn

import (
	"net/http"

	"github.com/obot-platform/obot/pkg/api/authz"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type Anonymous struct {
}

func (n Anonymous) AuthenticateRequest(*http.Request) (*authenticator.Response, bool, error) {
	return &authenticator.Response{
		User: &user.DefaultInfo{
			UID:    "anonymous",
			Name:   "anonymous",
			Groups: []string{authz.UnauthenticatedGroup},
		},
	}, true, nil
}
