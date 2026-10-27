package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
)

func TestValidateClientConfigUpstreamScopes(t *testing.T) {
	for _, test := range []struct {
		name      string
		scope     string
		wantError bool
	}{
		{
			name:  "supported ordinary scope",
			scope: "openid",
		},
		{
			name:  "captured upstream scope",
			scope: client.UpstreamOAuthScopePrefix + "nzcqyjmmgaqizmmedm4yzv2ojs",
		},
		{
			name:  "supported and unknown temporary scopes",
			scope: "openid " + client.UpstreamOAuthScopePrefix + "unknown",
		},
		{
			name:      "unsupported ordinary scope",
			scope:     "admin",
			wantError: true,
		},
		{
			name:      "temporary scope does not permit unsupported scopes",
			scope:     client.UpstreamOAuthScopePrefix + "pending admin",
			wantError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			oauthClient := &v1.OAuthClient{
				Spec: v1.OAuthClientSpec{
					Manifest: types.OAuthClientManifest{
						RedirectURIs: []string{"https://obot.test/tester"},
						Scope:        test.scope,
					},
				},
			}

			err := ValidateClientConfig(oauthClient, OAuthAuthorizationServerConfig{ScopesSupported: []string{"openid"}})
			if test.wantError {
				require.ErrorContains(t, err, "scope admin is not supported")
			} else {
				require.NoError(t, err)
			}
		})
	}
}
