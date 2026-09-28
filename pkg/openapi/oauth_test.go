package openapi

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestOAuthDoesNotBlockHeaderSuggestions(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		data := documentWith(t, func(doc map[string]any) {
			schemes := map[string]any{
				"oauth": map[string]any{"type": "oauth2", "flows": map[string]any{}},
			}
			if mixed {
				schemes["key"] = map[string]any{"type": "apiKey", "in": "header", "name": "X-API-Key"}
				schemes["token"] = map[string]any{"type": "http", "scheme": "bearer"}
			}
			doc["components"] = map[string]any{"securitySchemes": schemes}
			doc["security"] = []any{map[string]any{"oauth": []any{"read:users"}}}
		})
		result, err := Parse(data, types.OpenAPIRuntimeConfig{})
		require.NoError(t, err)
		require.JSONEq(t, string(data), string(result.Schema), "OAuth requirements must remain in the snapshot")
		if mixed {
			require.Len(t, result.SuggestedHeaders, 2)
			require.Equal(t, "X-API-Key", result.SuggestedHeaders[0].Key)
			require.Equal(t, "Authorization", result.SuggestedHeaders[1].Key)
			require.Equal(t, "Bearer ", result.SuggestedHeaders[1].Prefix)
		} else {
			require.Empty(t, result.SuggestedHeaders)
		}
		_, err = SettingsJSON(types.OpenAPIRuntimeConfig{}, result, result.SuggestedHeaders)
		require.NoError(t, err)
		if mixed {
			settings, err := SettingsJSON(types.OpenAPIRuntimeConfig{}, result, result.SuggestedHeaders[1:])
			require.NoError(t, err, "users may keep only the bearer header")
			require.Contains(t, string(settings), `"credentialHeaders":["Authorization"]`)
		}
	}
}
