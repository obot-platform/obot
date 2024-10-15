package authn

import (
	"net/http"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type Authenticator struct {
	authenticator authenticator.Request
}

func NewAuthenticator(authenticator authenticator.Request) *Authenticator {
	return &Authenticator{
		authenticator: authenticator,
	}
}

func (a *Authenticator) Authenticate(req *http.Request) (user.Info, error) {
	resp, ok, err := a.authenticator.AuthenticateRequest(req)
	if err != nil {
		return nil, err
	}
	if !ok {
		panic("authentication should always succeed")
	}
	return resp.User, nil
}
