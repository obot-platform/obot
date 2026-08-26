package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	obotOAuthClientNameForTest = "Obot MCP OAuth"
)

func TestClientNameCandidates(t *testing.T) {
	assert.Equal(t, []string{"Obot MCP OAuth"}, clientNameCandidates("Obot MCP OAuth", ""))
	assert.Equal(t, []string{"Obot MCP OAuth", "Claude Code"}, clientNameCandidates("Obot MCP OAuth", "Claude Code"))
	assert.Equal(t, []string{"Obot MCP OAuth"}, clientNameCandidates("Obot MCP OAuth", "Obot MCP OAuth"))
}

func TestResolveClientInfoClientNameFallback(t *testing.T) {
	const allowlistedClientName = "Claude Code"

	var (
		attemptedNames []string
		responseStatus = http.StatusForbidden
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var registration struct {
			ClientName string `json:"client_name"`
		}
		if err := json.Unmarshal(body, &registration); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		attemptedNames = append(attemptedNames, registration.ClientName)

		if registration.ClientName != allowlistedClientName {
			w.WriteHeader(responseStatus)
			_, _ = w.Write([]byte("Forbidden"))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"client_id":     "issued-client-id",
			"client_secret": "secret",
		})
	}))
	defer server.Close()

	discovery := oauthMetadataDiscovery{
		ProtectedResourceMetadata: protectedResourceMetadata{
			AuthorizationServers: []string{"https://auth.example.com"},
		},
		AuthorizationServerMetadata: AuthorizationServerMetadata{
			RegistrationEndpoint: server.URL,
		},
		ClientRegistration: ClientRegistrationMetadata{
			ClientName:   obotOAuthClientNameForTest,
			RedirectURIs: []string{"https://obot.example.com/oauth/mcp/callback"},
		},
	}

	t.Run("uses only primary name without fallback", func(t *testing.T) {
		attemptedNames = nil
		gatewayDiscovery := discovery
		gatewayDiscovery.ClientRegistration.ClientName = "Cursor"
		oauthClient := &oauth{
			metadataClient: server.Client(),
		}

		_, err := oauthClient.resolveClientInfo(context.Background(), "test-server", gatewayDiscovery)
		require.Error(t, err)
		assert.Equal(t, []string{"Cursor"}, attemptedNames)
	})

	t.Run("retries rejected primary name with fallback", func(t *testing.T) {
		attemptedNames = nil
		responseStatus = http.StatusForbidden
		oauthClient := &oauth{
			metadataClient:     server.Client(),
			clientNameFallback: allowlistedClientName,
		}

		clientInfo, err := oauthClient.resolveClientInfo(context.Background(), "test-server", discovery)
		require.NoError(t, err)
		assert.Equal(t, "issued-client-id", clientInfo.ClientID)
		assert.Equal(t, []string{obotOAuthClientNameForTest, allowlistedClientName}, attemptedNames)
	})

	t.Run("does not retry hard registration error", func(t *testing.T) {
		attemptedNames = nil
		responseStatus = http.StatusInternalServerError
		oauthClient := &oauth{
			metadataClient:     server.Client(),
			clientNameFallback: allowlistedClientName,
		}

		_, err := oauthClient.resolveClientInfo(context.Background(), "test-server", discovery)
		require.Error(t, err)
		assert.Equal(t, []string{obotOAuthClientNameForTest}, attemptedNames)
	})
}
