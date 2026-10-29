package persistent

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/stretchr/testify/require"
)

func TestCompositeTokenBoundsAPIKeyNamesBeforeLoopback(t *testing.T) {
	service, client := newLifecycleTestTokenService(t)
	owner := createLifecycleTestUser(t, client, "large-key-name-user", types.RoleBasic)
	masked := principal.MaskedAPIKeyName(fmt.Sprint(owner.ID), 42)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, ok, err := service.AuthenticateRequest(req)
		if err != nil || !ok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	for _, tt := range []struct {
		name      string
		auditName string
		rawName   string
		wantAudit string
		wantRaw   string
		refresh   bool
	}{
		{
			name:      "512 KiB name with injection disabled",
			auditName: strings.Repeat("a", 512<<10),
			rawName:   strings.Repeat("a", 512<<10),
			wantAudit: masked,
		},
		{
			name:      "ordinary name",
			auditName: "writer",
			rawName:   "writer",
			wantAudit: "writer",
			wantRaw:   "writer",
		},
		{
			name:      "unnamed key keeps masked audit name",
			auditName: masked,
			wantAudit: masked,
		},
		{
			name:      "512 Unicode characters fit",
			auditName: strings.Repeat("界", 512),
			rawName:   strings.Repeat("界", 512),
			wantAudit: strings.Repeat("界", 512),
			wantRaw:   strings.Repeat("界", 512),
		},
		{
			name:      "513 Unicode characters are oversized",
			auditName: strings.Repeat("界", 513),
			rawName:   strings.Repeat("界", 513),
			wantAudit: masked,
		},
		{
			name:      "oversized audit name only",
			auditName: strings.Repeat("a", 513),
			rawName:   "writer",
			wantAudit: masked,
			wantRaw:   "writer",
		},
		{
			name:      "oversized raw name only",
			auditName: "writer",
			rawName:   strings.Repeat("a", 513),
			wantAudit: "writer",
		},
		{
			name:      "refresh of an already issued unbounded token",
			auditName: strings.Repeat("a", 513),
			rawName:   strings.Repeat("a", 513),
			wantAudit: masked,
			refresh:   true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			claims := TokenContext{
				Audience:         testServerURL + "/mcp-connect-component",
				ExpiresAt:        NewTime(time.Now().Add(time.Hour)),
				UserID:           fmt.Sprint(owner.ID),
				UserGroups:       []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated},
				MCPID:            "vmcp",
				AuthorizedMCPIDs: []string{"component"},
				APIKeyID:         "42",
				APIKeyName:       tt.auditName,
				APIKeyRawName:    tt.rawName,
			}
			if tt.refresh {
				// Simulate a token issued before the signer imposed a name limit.
				claims.Issuer = testServerURL
				legacy, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(service.privateKey)
				require.NoError(t, err)
				decoded, err := service.DecodeToken(t.Context(), legacy)
				require.NoError(t, err)
				claims = *decoded
			}
			_, token, err := service.NewToken(t.Context(), claims)
			require.NoError(t, err)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, upstream.URL, nil)
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := upstream.Client().Do(req)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusNoContent, resp.StatusCode, "names must be bounded before the HTTP hop")
			require.Less(t, len(token), 16<<10)
			decoded, err := service.DecodeToken(t.Context(), token)
			require.NoError(t, err)
			require.Equal(t, "42", decoded.APIKeyID)
			require.Equal(t, tt.wantAudit, decoded.APIKeyName)
			require.Equal(t, tt.wantRaw, decoded.APIKeyRawName)
			require.Equal(t, claims.Audience, decoded.Audience)
			require.Equal(t, claims.UserID, decoded.UserID)
			require.Equal(t, claims.UserGroups, decoded.UserGroups)
			require.Equal(t, claims.AuthorizedMCPIDs, decoded.AuthorizedMCPIDs)
			require.Equal(t, tt.auditName, claims.APIKeyName, "signing must not mutate the caller's context")
			require.Equal(t, tt.rawName, claims.APIKeyRawName)
		})
	}
}

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
