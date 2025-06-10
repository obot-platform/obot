package wellknown

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

// oauthAuthorization handles the /.well-known/oauth-authorization-server endpoint
func (h *handler) oauthAuthorization(req api.Context) error {
	serverConfig := h.config

	if oauthAppID := req.PathValue("oauth_id"); oauthAppID != "" {
		var oauthApp v1.OAuthApp
		if err := req.Get(&oauthApp, req.PathValue("oauth_id")); err != nil {
			return err
		}

		serverConfig.AuthorizationEndpoint = fmt.Sprintf("%s/%s", serverConfig.AuthorizationEndpoint, oauthApp.Name)
	}

	return req.Write(serverConfig)
}
