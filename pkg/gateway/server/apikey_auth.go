package server

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/gateway/client"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const apiKeyAuthPrefix = "ok1-"

// APIKeyAuthenticator authenticates requests using API keys.
// API key users have restricted access - they only get GroupAPIKey,
// not the full authenticated user groups.
type APIKeyAuthenticator struct {
	client *client.Client
}

// NewAPIKeyAuthenticator creates a new API key authenticator.
func NewAPIKeyAuthenticator(client *client.Client) *APIKeyAuthenticator {
	return &APIKeyAuthenticator{client: client}
}

// AuthenticateRequest implements authenticator.Request.
func (a *APIKeyAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	authHeader := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if authHeader == "" {
		authHeader = req.Header.Get("X-API-Key")
		if authHeader == "" {
			return nil, false, nil
		}
	}

	// Check if this is an API key (starts with ok1-)
	if !strings.HasPrefix(authHeader, apiKeyAuthPrefix) {
		return nil, false, nil
	}

	// Validate the API key
	apiKey, err := a.client.ValidateAPIKey(req.Context(), authHeader)
	if err != nil {
		// Return false, nil to let other authenticators try
		// This allows the chain to continue if the key is invalid
		return nil, false, nil
	}

	// Get the user from the database
	u, err := a.client.UserByID(req.Context(), fmt.Sprintf("%d", apiKey.UserID))
	if err != nil {
		return nil, false, nil
	}

	extra := map[string][]string{
		"email": {u.Email},
	}

	// Look up auth provider group memberships so that group-based access
	// rules (e.g. skill access policies) work for API-key-authenticated
	// requests such as those made by nanobot.
	if authGroupIDs, err := a.client.ListGroupIDsForUser(req.Context(), u.ID); err == nil {
		extra["auth_provider_groups"] = authGroupIDs
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   u.Username,
			UID:    fmt.Sprintf("%d", u.ID),
			Groups: apiKey.Groups(u),
			Extra:  extra,
		},
	}, true, nil
}
