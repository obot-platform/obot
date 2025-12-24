package server

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type gatewayTokenReview struct {
	gatewayClient *client.Client
	gptClient     *gptscript.GPTScript
	dispatcher    *dispatcher.Dispatcher
}

func NewGatewayTokenReviewer(gatewayClient *client.Client, gptClient *gptscript.GPTScript, dispatcher *dispatcher.Dispatcher) authenticator.Request {
	return &gatewayTokenReview{
		gatewayClient: gatewayClient,
		gptClient:     gptClient,
		dispatcher:    dispatcher,
	}
}

func (g *gatewayTokenReview) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	bearer := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if bearer == "" {
		bearer = req.Header.Get("X-API-Key")
		if bearer == "" {
			return nil, false, nil
		}
	}

	u, namespace, name, providerUserID, groupIDs, err := g.gatewayClient.UserFromToken(req.Context(), bearer)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}

	if err := populateContext(req, g.gptClient, g.dispatcher, namespace, name); err != nil {
		return nil, false, err
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name: u.Username,
			UID:  providerUserID,
			Extra: map[string][]string{
				"email":                   {u.Email},
				"auth_provider_namespace": {namespace},
				"auth_provider_name":      {name},
				"auth_provider_groups":    groupIDs,
			},
		},
	}, true, nil
}

func populateContext(req *http.Request, gptClient *gptscript.GPTScript, dispatcher *dispatcher.Dispatcher, namespace, name string) error {
	providerURL, err := dispatcher.URLForAuthProvider(req.Context(), gptClient, namespace, name)
	if err != nil {
		return err
	}

	// Store the provider URL in context for later group fetching
	*req = *req.WithContext(auth.ContextWithProviderURL(req.Context(), providerURL.String()))

	return nil
}
