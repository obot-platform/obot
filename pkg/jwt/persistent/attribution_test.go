package persistent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/stretchr/testify/require"
)

func TestCompositeAttributionSurvivesSignedTokenRoundTrip(t *testing.T) {
	service, client := newLifecycleTestTokenService(t)
	owner := createLifecycleTestUser(t, client, "attributed-user", types.RoleBasic)
	for _, composite := range []bool{true, false} {
		t.Run(fmt.Sprint(composite), func(t *testing.T) {
			groups := StringSlice{types.GroupMCP, types.GroupAuthenticated}
			if composite {
				groups = append(groups, types.GroupCompositeMCP)
			}
			claims := TokenContext{
				Audience:      testServerURL + "/mcp-connect-component",
				IssuedAt:      NewTime(time.Now()),
				ExpiresAt:     NewTime(time.Now().Add(time.Hour)),
				UserID:        fmt.Sprint(owner.ID),
				UserName:      owner.Username,
				UserGroups:    groups,
				MCPID:         "vmcp",
				APIKeyID:      "42",
				APIKeyName:    "writer",
				APIKeyRawName: "writer",
			}
			_, token, err := service.NewToken(t.Context(), claims)
			require.NoError(t, err)
			decoded, err := service.DecodeToken(t.Context(), token)
			require.NoError(t, err)
			require.Equal(t, claims.APIKeyID, decoded.APIKeyID)
			require.Equal(t, claims.APIKeyName, decoded.APIKeyName)
			require.Equal(t, claims.APIKeyRawName, decoded.APIKeyRawName)
			req := httptest.NewRequest(http.MethodPost, testServerURL+"/mcp-connect-component", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			response, ok, err := service.AuthenticateRequest(req)
			require.NoError(t, err)
			require.True(t, ok)
			attribution, hasKey := principal.APIKeyAttributionFromUser(response.User)
			require.Equal(t, composite, hasKey, "ordinary tokens must not propagate key attribution")
			if composite {
				require.Equal(t, uint(42), attribution.ID)
				require.Equal(t, "writer", attribution.Name)
				require.Equal(t, []string{"writer"}, response.User.GetExtra()[principal.APIKeyRawNameExtra])
			}
		})
	}
}
