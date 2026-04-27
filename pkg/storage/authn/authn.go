package authn

import (
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/storage/authz"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

var (
	adminUserResponse = &authenticator.Response{
		User: &user.DefaultInfo{
			Name: authz.AdminName,
			UID:  authz.AdminName,
			Groups: []string{
				authz.AuthenticatedGroup,
				authz.AdminGroup,
			},
		},
	}
)

type Authenticator struct {
	authToken string
}

func NewAuthenticator(authToken string) *Authenticator {
	return &Authenticator{
		authToken: authToken,
	}
}

func (a *Authenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	bearerToken, ok := strings.CutPrefix(req.Header.Get("Authorization"), "Bearer ")
	bearerToken = strings.TrimSpace(bearerToken)
	if !ok || bearerToken == "" {
		return nil, false, nil
	}

	if bearerToken == a.authToken {
		return adminUserResponse, true, nil
	}

	return nil, false, nil
}
