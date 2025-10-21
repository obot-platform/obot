package setup

import (
	"net/http"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
)

type TempUserInfoResponse struct {
	UserID                uint       `json:"userId"`
	Username              string     `json:"username"`
	Email                 string     `json:"email"`
	Role                  types.Role `json:"role"`
	Groups                []string   `json:"groups"`
	IconURL               string     `json:"iconUrl,omitempty"`
	AuthProviderName      string     `json:"authProviderName"`
	AuthProviderNamespace string     `json:"authProviderNamespace"`
	CachedAt              string     `json:"cachedAt"`
}

// GetTempUser returns information about the temporarily cached user.
// Endpoint: GET /api/setup/temp-user
func (h *Handler) GetTempUser(req api.Context) error {
	if err := h.requireBootstrapEnabled(req); err != nil {
		return err
	}

	if err := h.requireBootstrap(req); err != nil {
		return err
	}

	cached := req.GatewayClient.GetTempUserCache(req.Context())
	if cached == nil {
		return types.NewErrHTTP(http.StatusNotFound, "no temporary user cached")
	}

	return req.Write(TempUserInfoResponse{
		UserID:                cached.UserID,
		Username:              cached.Username,
		Email:                 cached.Email,
		Role:                  cached.Role,
		Groups:                cached.Role.Groups(),
		IconURL:               cached.IconURL,
		AuthProviderName:      cached.AuthProviderName,
		AuthProviderNamespace: cached.AuthProviderNamespace,
		CachedAt:              cached.CreatedAt.Format(time.RFC3339),
	})
}
