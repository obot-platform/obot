package openapi

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestCredentials(t *testing.T) {
	for _, test := range []struct {
		name    string
		scheme  map[string]any
		key     string
		prefix  string
		invalid bool
	}{
		{
			name:   "header key",
			scheme: map[string]any{"type": "apiKey", "in": "header", "name": "X-API-Key"},
			key:    "X-API-Key",
		},
		{
			name:   "bearer token",
			scheme: map[string]any{"type": "http", "scheme": "bearer"},
			key:    "Authorization",
			prefix: "Bearer ",
		},
		{
			name:    "query key",
			scheme:  map[string]any{"type": "apiKey", "in": "query", "name": "key"},
			invalid: true,
		},
		{
			name:    "cookie key",
			scheme:  map[string]any{"type": "apiKey", "in": "cookie", "name": "key"},
			invalid: true,
		},
		{
			name:    "transport header",
			scheme:  map[string]any{"type": "apiKey", "in": "header", "name": "Host"},
			invalid: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := documentWith(t, func(d map[string]any) {
				d["components"] = map[string]any{"securitySchemes": map[string]any{"key": test.scheme}}
			})
			result, err := Parse(data, types.OpenAPIRuntimeConfig{})
			if test.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Len(t, result.SuggestedHeaders, 1)
			header := result.SuggestedHeaders[0]
			require.Equal(t, test.key, header.Key)
			require.Equal(t, test.prefix, header.Prefix)
			require.True(t, header.Sensitive)
			require.True(t, header.Required)
			settingsWithoutHeaders, err := SettingsJSON(result, nil)
			require.NoError(t, err, "suggested headers may be removed")
			require.JSONEq(t, `{"baseURL":"https://api.example.com/v1/","credentialHeaders":[]}`, string(settingsWithoutHeaders))
			header.Value = "must-not-be-serialized"
			settings, err := SettingsJSON(result, []types.MCPConfig{header})
			require.NoError(t, err)
			require.NotContains(t, string(settings), header.Value)
			require.NotContains(t, string(settings), "Bearer ")
			require.JSONEq(t, `{"baseURL":"https://api.example.com/v1/","credentialHeaders":["`+test.key+`"]}`, string(settings))
			_, err = Parse(data, types.OpenAPIRuntimeConfig{BaseURL: "http://api.example.com"})
			require.ErrorContains(t, err, "HTTPS")
		})
	}
}

func TestSettingsValidation(t *testing.T) {
	result, err := Parse(usersSchema(t), types.OpenAPIRuntimeConfig{})
	require.NoError(t, err)
	header := types.MCPConfig{
		Key:       "X-Key",
		Usage:     types.Header,
		Sensitive: true,
		Required:  true,
	}
	for _, key := range []string{"Host", "Cookie", "Mcp-Session-Id", "Proxy-Token", "Sec-Fetch-Site", "invalid header", "x\r\nInjected: value"} {
		bad := header
		bad.Key = key
		_, err := SettingsJSON(result, []types.MCPConfig{bad})
		require.Error(t, err)
	}
	duplicate := header
	duplicate.Key = "x-key"
	_, err = SettingsJSON(result, []types.MCPConfig{header, duplicate})
	require.ErrorContains(t, err, "duplicate")
	bad := header
	bad.Usage = types.Env
	_, err = SettingsJSON(result, []types.MCPConfig{bad})
	require.ErrorContains(t, err, "must be header inputs")
	for _, required := range []bool{true, false} {
		for _, sensitive := range []bool{true, false} {
			edited := header
			edited.Required = required
			edited.Sensitive = sensitive
			settings, err := SettingsJSON(result, []types.MCPConfig{edited})
			require.NoError(t, err)
			require.Contains(t, string(settings), `"credentialHeaders":["X-Key"]`)
		}
	}
	header.Key = strings.Repeat("a", MaxSettingsBytes)
	_, err = SettingsJSON(result, []types.MCPConfig{header})
	require.ErrorContains(t, err, "96 KiB")

	for _, base := range []string{"/relative", "https://user:pass@api.example.com", "https://api.example.com?a=b", "https://api.example.com#fragment", "http://localhost", "http://127.0.0.1", "http://[::ffff:127.0.0.1]", "http://169.254.169.254", "https://{host}/api"} {
		_, err := Parse(usersSchema(t), types.OpenAPIRuntimeConfig{BaseURL: base})
		require.Error(t, err)
	}
}

func TestReferencesAndParameters(t *testing.T) {
	for _, test := range []struct {
		name      string
		parameter map[string]any
		invalid   bool
	}{
		{
			name:      "local parameter",
			parameter: map[string]any{"name": "page", "in": "query", "schema": map[string]any{"type": "integer"}},
		},
		{
			name:      "cookie parameter",
			parameter: map[string]any{"name": "cookie", "in": "cookie"},
			invalid:   true,
		},
		{
			name:      "transport parameter",
			parameter: map[string]any{"name": "Host", "in": "header"},
			invalid:   true,
		},
		{
			name:      "cyclic parameter",
			parameter: map[string]any{"$ref": "#/components/parameters/page"},
			invalid:   true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := documentWith(t, func(d map[string]any) {
				d["components"] = map[string]any{"parameters": map[string]any{"page": test.parameter}}
				d["paths"].(map[string]any)["/users"].(map[string]any)["parameters"] = []any{map[string]any{"$ref": "#/components/parameters/page"}}
			})
			_, err := Parse(data, types.OpenAPIRuntimeConfig{})
			if test.invalid {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
	// Recursive data models are valid; the importer does not expand them.
	data := documentWith(t, func(d map[string]any) {
		d["components"] = map[string]any{"schemas": map[string]any{"node": map[string]any{"$ref": "#/components/schemas/node"}}}
	})
	_, err := Parse(data, types.OpenAPIRuntimeConfig{})
	require.NoError(t, err)
}
