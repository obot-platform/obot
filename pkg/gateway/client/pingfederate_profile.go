package client

import "github.com/obot-platform/obot/pkg/gateway/types"

func applyPingFederateProfile(user *types.User, identity *types.Identity, profile map[string]any) {
	if name, ok := profile["name"].(string); ok && name != "" {
		user.DisplayName = name
	}
	if picture, ok := profile["picture"].(string); ok {
		user.IconURL = picture
		identity.IconURL = picture
	}
	user.EmailDisplayDomain, _ = profile["email_display_domain"].(string)
}
