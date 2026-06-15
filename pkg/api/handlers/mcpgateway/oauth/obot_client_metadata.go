package oauth

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/system"
)

func (h *handler) obotClientIDMetadata(req api.Context) error {
	return req.Write(clientIDMetadataDocument{
		ClientID: system.OAuthClientIDMetadataURL(h.baseURL),
		OAuthClientManifest: types.OAuthClientManifest{
			RedirectURIs:            []string{system.MCPOAuthCallbackURL(h.baseURL)},
			TokenEndpointAuthMethod: "none",
			GrantTypes:              []string{"authorization_code", "refresh_token"},
			ResponseTypes:           []string{"code"},
			ClientName:              "Obot MCP Gateway",
			ClientURI:               h.baseURL,
			SoftwareID:              "obot",
		},
	})
}
