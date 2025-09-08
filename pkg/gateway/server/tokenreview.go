package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/nanobot-ai/nanobot/pkg/log"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	cookiePrefix = auth.ObotAccessTokenCookie + "="
)

type gatewayTokenReview struct {
	gatewayClient *client.Client
	dispatcher    *dispatcher.Dispatcher
}

func NewGatewayTokenReviewer(gatewayClient *client.Client, dispatcher *dispatcher.Dispatcher) authenticator.Request {
	return &gatewayTokenReview{
		gatewayClient: gatewayClient,
		dispatcher:    dispatcher,
	}
}

func (g *gatewayTokenReview) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	bearer := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if bearer == "" {
		return nil, false, nil
	}

	u, namespace, name, providerUserID, hashedSessionID, groupIDs, err := g.gatewayClient.UserFromToken(req.Context(), bearer)
	if err != nil {
		return nil, false, err
	}

	if hashedSessionID != "" {
		err := HandleHashedSessionID(req, g.gatewayClient, g.dispatcher, hashedSessionID, namespace, name)
		if err != nil {
			return nil, false, err
		}
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

func HandleHashedSessionID(req *http.Request, gatewayClient *client.Client, dispatcher *dispatcher.Dispatcher, hashedSessionID, namespace, name string) error {
	// Grab the access token from the session cookie and ask the auth provider for the IdP's access token.
	sessionCookie, err := gatewayClient.GetSessionCookie(req.Context(), hashedSessionID, namespace, name)
	if err != nil {
		return err
	}

	providerURL, err := dispatcher.URLForAuthProvider(req.Context(), namespace, name)
	if err != nil {
		return err
	}

	// Get the session state from the auth provider,
	ss, err := getSessionState(req, providerURL.String(), sessionCookie.Cookie)
	if err != nil {
		// On failure, delete the session cookie (which also deletes tokens for the session).
		if err := gatewayClient.DeleteSessionCookie(req.Context(), hashedSessionID, namespace, name); err != nil {
			log.Errorf(req.Context(), "failed to delete session cookie: %v", err)
		}
		return err
	}

	// Check if the auth provider refreshed the session cookie.
	var newCookie string
	for _, setCookie := range ss.SetCookies {
		if _, newCookie, _ = strings.Cut(setCookie, cookiePrefix); setCookie != "" {
			break
		}
	}

	if newCookie != "" && newCookie != sessionCookie.Cookie {
		// Provider refreshed the session cookie, update the cached cookie.
		sessionCookie.Cookie = newCookie
		if err := gatewayClient.EnsureSessionCookie(req.Context(), *sessionCookie); err != nil {
			return err
		}
	}

	*req = *req.WithContext(accesstoken.ContextWithAccessToken(req.Context(), ss.AccessToken))
	*req = *req.WithContext(auth.ContextWithProviderURL(req.Context(), providerURL.String()))
	return nil
}

func getSessionState(req *http.Request, authProviderURL, cookie string) (*auth.SerializableState, error) {
	// Clone the header to avoid modifying the original request
	header := req.Header.Clone()

	// Add the cookie to the header
	header.Set("Cookie", cookiePrefix+cookie)

	sr := auth.SerializableRequest{
		Method: req.Method,
		URL:    req.URL.String(),
		Header: header,
	}

	srJSON, err := json.Marshal(sr)
	if err != nil {
		return nil, err
	}

	stateRequest, err := http.NewRequest(http.MethodPost, authProviderURL+"/obot-get-state", strings.NewReader(string(srJSON)))
	if err != nil {
		return nil, err
	}

	stateResponse, err := http.DefaultClient.Do(stateRequest)
	if err != nil {
		return nil, err
	}
	defer stateResponse.Body.Close()

	var ss auth.SerializableState
	if err = json.NewDecoder(stateResponse.Body).Decode(&ss); err != nil {
		return nil, err
	}

	return &ss, nil
}
