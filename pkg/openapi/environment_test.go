package openapi

import (
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestEnvironment(t *testing.T) {
	result := &Result{BaseURL: "https://api.example.com/v1"}
	env, err := Environment(result, nil)
	require.NoError(t, err)
	require.Equal(t, []string{
		"OPENAPI_BASE_URL=https://api.example.com/v1/",
		"OPENAPI_CREDENTIAL_HEADERS=",
	}, env)

	headers := []types.MCPConfig{
		{Key: "X-Key", Usage: types.Header, Value: "secret"},
		{Key: "Authorization", Usage: types.Header, Prefix: "Bearer "},
	}
	env, err = Environment(result, headers)
	require.NoError(t, err)
	require.Equal(t, []string{
		"OPENAPI_BASE_URL=https://api.example.com/v1/",
		"OPENAPI_CREDENTIAL_HEADERS=X-Key,Authorization",
	}, env)
	require.NotContains(t, strings.Join(env, "\n"), "secret")
	require.NotContains(t, strings.Join(env, "\n"), "Bearer ")
}

func TestEnvironmentValidation(t *testing.T) {
	result := &Result{BaseURL: "https://api.example.com"}
	header := types.MCPConfig{Key: "X-Key", Usage: types.Header}

	for _, key := range []string{"Host", "Cookie", "Mcp-Session-Id", "invalid header", "x\r\nInjected: value"} {
		bad := header
		bad.Key = key
		_, err := Environment(result, []types.MCPConfig{bad})
		require.Error(t, err)
	}

	duplicate := header
	duplicate.Key = "x-key"
	_, err := Environment(result, []types.MCPConfig{header, duplicate})
	require.ErrorContains(t, err, "duplicate")

	bad := header
	bad.Usage = types.Env
	_, err = Environment(result, []types.MCPConfig{bad})
	require.ErrorContains(t, err, "header inputs")

	_, err = Environment(&Result{BaseURL: "http://api.example.com"}, []types.MCPConfig{header})
	require.ErrorContains(t, err, "HTTPS")

	_, err = Environment(&Result{
		BaseURL:          "https://api.example.com",
		SuggestedHeaders: []types.MCPConfig{{Key: "X-Required", Usage: types.Header}},
	}, []types.MCPConfig{header})
	require.ErrorContains(t, err, "X-Required")

	header.Key = strings.Repeat("a", maxCredentialHeadersBytes+1)
	_, err = Environment(result, []types.MCPConfig{header})
	require.ErrorContains(t, err, "96 KiB")
}
