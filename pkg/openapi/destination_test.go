package openapi

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/safehttp"
	"github.com/stretchr/testify/require"
)

func TestImportBeforeConfiguringDestination(t *testing.T) {
	for _, serverURL := range []string{"", "/api/v3", "//petstore.swagger.io/v2"} {
		t.Run(serverURL, func(t *testing.T) {
			data := documentWith(t, func(doc map[string]any) {
				delete(doc, "servers")
				if serverURL != "" {
					doc["servers"] = []any{map[string]any{"url": serverURL}}
				}
				doc["components"] = map[string]any{"securitySchemes": map[string]any{
					"token": map[string]any{"type": "http", "scheme": "bearer"},
				}}
			})
			result, err := Parse(data, types.OpenAPIRuntimeConfig{})
			require.NoError(t, err)
			require.Empty(t, result.BaseURL)
			require.NotEmpty(t, result.Schema)
			require.Equal(t, "Users", result.SuggestedMetadata.Name)
			require.Len(t, result.SuggestedHeaders, 1)
			_, err = Environment(result, result.SuggestedHeaders, false)
			require.ErrorContains(t, err, "configure baseURL")
			config := types.OpenAPIRuntimeConfig{Schema: &types.OpenAPISchema{Raw: result.Schema}}
			_, err = SnapshotEnvironment(config, result.SuggestedHeaders, false)
			require.ErrorContains(t, err, "configure baseURL")
			_, err = ValidateSnapshotEnvironment(t.Context(), config, result.SuggestedHeaders, safehttp.Options{}, false)
			require.EqualError(t, err, "no usable server URL; configure baseURL")
			config.BaseURL = "https://api.example.com/v3"
			_, err = SnapshotEnvironment(config, result.SuggestedHeaders, false)
			require.NoError(t, err)
			config.BaseURL = "https://8.8.8.8/v3"
			_, err = ValidateSnapshotEnvironment(t.Context(), config, result.SuggestedHeaders, safehttp.Options{}, false)
			require.NoError(t, err)
			config.BaseURL = "http://api.example.com/v3"
			_, err = SnapshotEnvironment(config, result.SuggestedHeaders, false)
			require.ErrorContains(t, err, "HTTPS")
		})
	}
}
