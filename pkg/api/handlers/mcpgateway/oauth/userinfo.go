package oauth

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	gtypes "github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/obot/pkg/api"
)

// UserInfoResponse represents the OpenID Connect UserInfo response
type UserInfoResponse struct {
	// Required claim - subject identifier
	Sub string `json:"sub"`

	// Profile scope claims
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Picture           string `json:"picture,omitempty"`
	Zoneinfo          string `json:"zoneinfo,omitempty"`
	UpdatedAt         int64  `json:"updated_at,omitempty"`

	// Email scope claims
	Email         string `json:"email,omitempty"`
	EmailVerified *bool  `json:"email_verified,omitempty"`
}

func (h *handler) userInfo(req api.Context) error {
	scope := gtypes.FirstSet(req.User.GetExtra()["oauthScope"]...)
	if !slices.Contains(strings.Fields(scope), "profile") {
		return h.writeUserInfoError(req, http.StatusUnauthorized,
			"invalid_scope", "Insufficient scope")
	}

	userID := req.User.GetUID()
	user, err := req.GatewayClient.UserByID(req.Context(), userID)
	if err != nil {
		// Don't reveal whether user exists - return generic error
		return h.writeUserInfoError(req, http.StatusForbidden, "invalid_token", "Invalid token")
	}

	response := &UserInfoResponse{
		Sub:               userID,
		PreferredUsername: user.Username,
		Picture:           user.IconURL,
		Zoneinfo:          user.Timezone,
		Email:             user.Email,
		EmailVerified:     user.VerifiedEmail,
	}

	// Use DisplayName as name, fallback to Username if DisplayName is empty
	if user.DisplayName != "" {
		response.Name = user.DisplayName
	} else {
		response.Name = user.Username
	}

	return req.Write(response)
}

// writeUserInfoError writes an OAuth 2.0 Bearer token error response per RFC 6750
func (h *handler) writeUserInfoError(req api.Context, statusCode int, errorCode, description string) error {
	// Set WWW-Authenticate header for 401 responses
	if statusCode == http.StatusUnauthorized {
		wwwAuth := fmt.Sprintf(`Bearer error="%s"`, errorCode)
		if description != "" {
			wwwAuth += fmt.Sprintf(`, error_description="%s"`, description)
		}
		req.ResponseWriter.Header().Set("WWW-Authenticate", wwwAuth)
	}

	// Return JSON error response
	req.ResponseWriter.Header().Set("Content-Type", "application/json")
	req.WriteHeader(statusCode)

	return req.Write(map[string]string{
		"error":             errorCode,
		"error_description": description,
	})
}
