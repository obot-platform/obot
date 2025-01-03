package bootstrap

import (
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/pkg/api/authz"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const bootstrapCookie = "obot-bootstrap"

type Bootstrap struct {
	token string
}

func New() (*Bootstrap, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate random token: %w", err)
	}

	fmt.Printf("Bootstrap token: %s\n", fmt.Sprintf("%x", bytes))

	return &Bootstrap{
		token: fmt.Sprintf("%x", bytes),
	}, nil
}

func (b *Bootstrap) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		// Check for the cookie.
		c, err := req.Cookie(bootstrapCookie)
		if err != nil || c.Value != b.token {
			return nil, false, nil
		}
	} else if authHeader != fmt.Sprintf("Bearer %s", b.token) {
		return nil, false, nil
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name: "bootstrap",
			UID:  "bootstrap",
			Groups: []string{
				authz.AdminGroup,
				authz.AuthenticatedGroup,
			},
		},
	}, true, nil
}
