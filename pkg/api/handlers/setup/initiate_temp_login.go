package setup

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
)

type InitiateTempLoginRequest struct {
	AuthProviderName string `json:"authProviderName"`
	AuthProviderNS   string `json:"authProviderNamespace"`
}

type InitiateTempLoginResponse struct {
	RedirectURL string `json:"redirectUrl"`
	TokenID     string `json:"tokenId"`
}

// InitiateTempLogin starts an OAuth flow for any user via the specified auth provider.
// The user does not need to be pre-configured as an Owner - any authenticated user
// can become the first Owner if the bootstrap user confirms them.
// Endpoint: POST /api/setup/initiate-temp-login
func (h *Handler) InitiateTempLogin(req api.Context) error {
	if err := h.requireBootstrap(req); err != nil {
		return err
	}

	var body InitiateTempLoginRequest
	if err := req.Read(&body); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	// Validate required fields
	if body.AuthProviderName == "" || body.AuthProviderNS == "" {
		return types.NewErrBadRequest("authProviderName and authProviderNamespace are required")
	}

	// Check if a temporary user is already cached
	if cached := h.gatewayClient.GetTempUserCache(); cached != nil {
		return types.NewErrHTTP(http.StatusConflict,
			fmt.Sprintf("temporary user already cached: %s", cached.Email))
	}

	// Create TokenRequest for OAuth flow with setup context
	tokenID := uuid.New().String()
	tokenRequest := &gwtypes.TokenRequest{
		ID:                    tokenID,
		CompletionRedirectURL: fmt.Sprintf("%s/api/setup/oauth-complete", req.APIBaseURL),
		ExpiresAt:             time.Now().Add(15 * time.Minute),
	}

	if err := h.gatewayClient.CreateTokenRequest(req.Context(), tokenRequest); err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	// Build OAuth start URL
	redirectURL := fmt.Sprintf("%s/api/oauth/start/%s/%s/%s",
		req.APIBaseURL,
		tokenID,
		body.AuthProviderNS,
		body.AuthProviderName,
	)

	return req.Write(InitiateTempLoginResponse{
		RedirectURL: redirectURL,
		TokenID:     tokenID,
	})
}
