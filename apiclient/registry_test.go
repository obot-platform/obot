package apiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestListRegistryServersUsesAppBaseURLAndAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/v0.1/servers", r.URL.Path)
		require.Equal(t, "github issues", r.URL.Query().Get("search"))
		require.Equal(t, "next", r.URL.Query().Get("cursor"))
		require.Equal(t, "100", r.URL.Query().Get("limit"))
		require.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		require.NoError(t, json.NewEncoder(w).Encode(types.RegistryServerList{
			Servers: []types.RegistryServerResponse{{
				Server: types.RegistryServerDetail{
					Name:        "io.example.github",
					Title:       "GitHub",
					Description: "GitHub MCP server",
				},
			}},
			Metadata: &types.RegistryServerListMetadata{NextCursor: "done", Count: 1},
		}))
	}))
	defer server.Close()

	result, err := (&Client{
		BaseURL: server.URL + "/api/",
		Token:   "test-token",
	}).ListRegistryServers(context.Background(), ListRegistryServersOptions{
		Search: "github issues",
		Cursor: "next",
		Limit:  100,
	})

	require.NoError(t, err)
	require.Len(t, result.Servers, 1)
	require.Equal(t, "io.example.github", result.Servers[0].Server.Name)
	require.Equal(t, "done", result.Metadata.NextCursor)
}

func TestListRegistryServersOmitsEmptyParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v0.1/servers", r.URL.Path)
		require.Equal(t, "", r.URL.RawQuery)
		require.NoError(t, json.NewEncoder(w).Encode(types.RegistryServerList{}))
	}))
	defer server.Close()

	result, err := (&Client{BaseURL: server.URL + "/api"}).ListRegistryServers(context.Background(), ListRegistryServersOptions{})
	require.NoError(t, err)
	require.Empty(t, result.Servers)
}
