package server

import (
	"fmt"
	"net/http"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
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
	// Extract Bearer token from Authorization header
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		return nil, false, nil
	}

	bearer := strings.TrimPrefix(authHeader, "Bearer ")
	if bearer == authHeader {
		// No "Bearer " prefix
		return nil, false, nil
	}

	// Check if this is an API key (starts with ok1-)
	if !strings.HasPrefix(bearer, apiKeyAuthPrefix) {
		return nil, false, nil
	}

	// Validate the API key
	apiKey, err := a.client.ValidateAPIKey(req.Context(), bearer)
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

	// IMPORTANT: API key users only get GroupAPIKey, not the full user groups.
	// This restricts them to MCP-connect routes and /api/me only.
	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   u.Username,
			UID:    fmt.Sprintf("%d", u.ID),
			Groups: []string{types2.GroupAPIKey},
		},
	}, true, nil
}
