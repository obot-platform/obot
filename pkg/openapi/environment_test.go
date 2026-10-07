package openapi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/safehttp"
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
		{Key: "X-Key", Required: true, Usage: types.Header, Value: "secret"},
		{Key: "Authorization", Required: true, Usage: types.Header, Prefix: "Bearer "},
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
	header := types.MCPConfig{Key: "X-Key", Required: true, Usage: types.Header}

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
		SuggestedHeaders: []types.MCPConfig{{Key: "X-Required", Required: true, Usage: types.Header}},
	}, []types.MCPConfig{header})
	require.ErrorContains(t, err, "X-Required")

	header.Key = strings.Repeat("a", maxCredentialHeadersBytes+1)
	_, err = Environment(result, []types.MCPConfig{header})
	require.ErrorContains(t, err, "96 KiB")
}

func TestSnapshotDestinationPolicy(t *testing.T) {
	for _, test := range []struct {
		name    string
		url     string
		options safehttp.Options
		devMode bool
		headers []types.MCPConfig
		wantErr string
	}{
		{
			name:    "production HTTP without credentials",
			url:     "http://93.184.216.34",
			wantErr: "API destination must use HTTPS",
		},
		{
			name:    "development HTTP",
			url:     "http://127.0.0.1:9999",
			devMode: true,
		},
		{
			name:    "development credentials still require HTTPS",
			url:     "http://127.0.0.1:9999",
			devMode: true,
			headers: []types.MCPConfig{{Key: "X-Key", Required: true, Usage: types.Header}},
			wantErr: "credential forwarding requires an HTTPS",
		},
		{
			name:    "loopback blocked",
			url:     "https://127.0.0.1",
			options: safehttp.Options{BlockLoopback: true},
			wantErr: "blocked loopback",
		},
		{
			name:    "unspecified blocked",
			url:     "https://0.0.0.0",
			options: safehttp.Options{BlockLoopback: true},
			wantErr: "blocked unspecified",
		},
		{
			name:    "private blocked",
			url:     "https://10.0.0.1",
			options: safehttp.Options{BlockPrivateIP: true},
			wantErr: "blocked private",
		},
		{
			name:    "link local blocked",
			url:     "https://169.254.169.254",
			options: safehttp.Options{BlockLinkLocal: true},
			wantErr: "blocked link-local",
		},
		{
			name: "operator allows private destination",
			url:  "https://10.0.0.1",
			options: safehttp.Options{
				BlockLoopback:  true,
				BlockLinkLocal: true,
			},
		},
		{
			name: "public destination",
			url:  "https://93.184.216.34",
			options: safehttp.Options{
				BlockLoopback:  true,
				BlockPrivateIP: true,
				BlockLinkLocal: true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, override := range []bool{false, true} {
				t.Run(fmt.Sprintf("override=%t", override), func(t *testing.T) {
					data := documentWith(t, func(d map[string]any) {
						delete(d, "components")
						d["servers"] = []any{map[string]any{"url": test.url}}
					})
					config := types.OpenAPIRuntimeConfig{
						Source: types.OpenAPISource{URL: "https://must-not-fetch.invalid/schema"},
						Schema: &types.OpenAPISchema{Raw: data},
					}
					if override {
						config.BaseURL = test.url
						config.Schema.Raw = documentWith(t, func(d map[string]any) {
							delete(d, "components")
						})
					}
					env, err := ValidateSnapshotEnvironment(t.Context(), config, test.headers, test.options, test.devMode)
					if test.wantErr != "" {
						require.ErrorContains(t, err, test.wantErr)
						require.Nil(t, env)
					} else {
						require.NoError(t, err)
						require.Equal(t, "OPENAPI_BASE_URL="+test.url+"/", env[0])
					}
				})
			}
		})
	}
}
