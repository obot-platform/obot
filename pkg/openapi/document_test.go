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
			result, err = Parse(data, types.OpenAPIRuntimeConfig{BaseURL: "http://api.example.com"})
			require.NoError(t, err)
			require.Equal(t, "http://api.example.com/", result.BaseURL)
			require.Len(t, result.SuggestedHeaders, 1)
		})
	}
}

func TestHeaderValidation(t *testing.T) {
	for _, key := range []string{"Host", "Cookie", "Mcp-Session-Id", "Proxy-Token", "Sec-Fetch-Site", "Forwarded", "X-Forwarded-For", "invalid header", "x\r\nInjected: value"} {
		require.Error(t, validateHeader(key))
	}
}

func TestBaseURLValidation(t *testing.T) {
	for _, base := range []string{"/relative", "https://user:pass@api.example.com", "https://api.example.com?a=b", "https://api.example.com#fragment", "https://{host}/api"} {
		_, err := Parse(usersSchema(t), types.OpenAPIRuntimeConfig{BaseURL: base})
		require.Error(t, err)
	}
	for _, base := range []string{"http://localhost", "http://127.0.0.1", "http://[::ffff:127.0.0.1]", "http://169.254.169.254", "https://10.0.0.1", "https://192.168.1.1", "https://[fd00::1]"} {
		result, err := Parse(usersSchema(t), types.OpenAPIRuntimeConfig{BaseURL: base})
		require.NoError(t, err)
		require.Equal(t, strings.TrimRight(base, "/")+"/", result.BaseURL)
	}
}

func TestURLPathSpacesAndControls(t *testing.T) {
	base, err := destination("https://api.example.com/my path")
	require.NoError(t, err)
	require.Equal(t, "https://api.example.com/my%20path/", base)

	for _, source := range []string{"https://api.example.com/a\rb", "https://api.example.com/a\tb", "https://api.example.com/a\\b"} {
		_, err := sourceURL(source)
		require.Error(t, err)
	}
}

func TestSchemaSourceHTTPSRequirement(t *testing.T) {
	_, err := schemaSourceURL("http://example.com/schema", false)
	require.ErrorContains(t, err, "HTTPS")

	httpSource, err := schemaSourceURL("http://example.com/schema", true)
	require.NoError(t, err)
	require.Equal(t, "http", httpSource.Scheme)

	httpsSource, err := schemaSourceURL("https://example.com/schema", false)
	require.NoError(t, err)
	require.Equal(t, "https", httpsSource.Scheme)
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
