package oauth

import (
	"net/url"
	"testing"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/require"
)

func TestAuthorizeRedirectPreservesQuery(t *testing.T) {
	for _, redirect := range []string{
		"https://obot.test/callback",
		"https://obot.test/callback?inspector=vmcp1&mcp_tester_oauth=%2Fmcp-connect%2Fvmcp1",
	} {
		t.Run(redirect, func(t *testing.T) {
			request := v1.OAuthAuthRequest{}
			request.Spec.RedirectURI = redirect
			request.Spec.State = "state with spaces"

			for _, success := range []bool{true, false} {
				result := authorizeResponseURL(request, "code+value", "read write")
				if !success {
					result = authorizeErrorURL(redirect, oauthError{
						Code:        ErrAccessDenied,
						Description: "denied",
						State:       request.Spec.State,
					})
				}

				parsed, err := url.Parse(result)
				require.NoError(t, err)
				original, err := url.Parse(redirect)
				require.NoError(t, err)
				for key, values := range original.Query() {
					require.Equal(t, values, parsed.Query()[key])
				}

				require.Equal(t, request.Spec.State, parsed.Query().Get("state"))
				if success {
					require.Equal(t, "code+value", parsed.Query().Get("code"))
					require.Equal(t, "read write", parsed.Query().Get("scope"))
				} else {
					require.Equal(t, string(ErrAccessDenied), parsed.Query().Get("error"))
					require.Equal(t, "denied", parsed.Query().Get("error_description"))
				}
			}
		})
	}
}
