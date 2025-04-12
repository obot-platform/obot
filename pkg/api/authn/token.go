package authn

import (
	"net/http"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type Token struct {
	Token, Username string
	Groups          []string
}

func NewToken(token, username string, groups ...string) *Token {
	return &Token{
		Token:    token,
		Username: username,
		Groups:   groups,
	}
}

func (t *Token) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	if req.Header.Get("Authorization") == "Bearer "+t.Token {
		return &authenticator.Response{
			User: &user.DefaultInfo{
				UID:    t.Username,
				Name:   t.Username,
				Groups: t.Groups,
			},
		}, true, nil
	}

	return nil, false, nil
}
