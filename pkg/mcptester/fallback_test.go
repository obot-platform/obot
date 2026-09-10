package mcptester

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/tidwall/gjson"
)

func TestParseModelProxyURL(t *testing.T) {
	tests := []struct {
		name        string
		value       string
		development bool
		want        string
		wantErr     bool
	}{
		{
			name:  "disabled",
			value: "",
		},
		{
			name:  "default",
			value: DefaultModelProxyURL,
			want:  "https://model-service.obot.ai/v1/responses",
		},
		{
			name:  "prefix",
			value: "https://example.com/proxy/",
			want:  "https://example.com/proxy/v1/responses",
		},
		{
			name:  "endpoint once",
			value: "https://example.com/proxy/v1/responses/",
			want:  "https://example.com/proxy/v1/responses",
		},
		{
			name:        "development HTTP",
			value:       "http://localhost:1234",
			development: true,
			want:        "http://localhost:1234/v1/responses",
		},
		{
			name:    "production HTTP",
			value:   "http://example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseModelProxyURL(tt.value, tt.development)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v", err)
			}

			var got string
			if parsed != nil {
				got = parsed.String()
			}

			if got != tt.want {
				t.Fatalf("URL = %q, want %q", got, tt.want)
			}
		})
	}

	for _, value := range []string{"/relative", "https://", "https://user:secret@example.com", "https://example.com?token=secret", "https://example.com?", "https://example.com#", "https://example.com#secret", "https://example.com:0", "https://example.com:65536", "https://example.com:", "ftp://example.com", "https://example.com:bad", " https://example.com", "https://exa_mple.com"} {
		t.Run(value, func(t *testing.T) {
			if _, err := ParseModelProxyURL(value, true); err == nil {
				t.Fatal("accepted invalid URL")
			}
		})
	}
}

func TestFallbackRequestContract(t *testing.T) {
	request := types.MCPTesterChatRequest{
		Round: 1,
		Messages: []types.MCPTesterChatMessage{{
			Role:    types.MCPTesterChatRoleUser,
			Content: []types.MCPTesterContent{{Type: types.MCPTesterContentTypeText, Text: "hello"}},
		}},
	}

	body, err := BuildFallbackRequest(request, "server instruction")
	if err != nil {
		t.Fatal(err)
	}

	for key, want := range map[string]string{"model": FallbackModel, "reasoning.effort": "high", "store": "false", "stream": "true", "max_output_tokens": "16384", "instructions": "server instruction"} {
		if got := gjson.GetBytes(body, key).String(); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}

	endpoint, err := ParseModelProxyURL(DefaultModelProxyURL, false)
	if err != nil {
		t.Fatal(err)
	}

	req, err := NewFallbackRequest(t.Context(), endpoint, body, "signed==.KEY", "machine-id", Descriptor{Header: descriptorCommandHeader, Value: "npx @org/package@1.0"})
	if err != nil {
		t.Fatal(err)
	}

	if req.Header.Get("Authorization") != "Bearer signed==.KEY" || req.Header.Get("X-Obot-Machine-Fingerprint") != "machine-id" || len(req.Header) != 6 || req.GetBody != nil {
		t.Fatalf("unexpected request: %#v", req)
	}

	if _, err := NewFallbackRequest(t.Context(), endpoint, body, "", "machine-id", Descriptor{}); err == nil {
		t.Fatal("accepted empty license")
	}

	request.Tools = []types.MCPTesterTool{{Name: "large", Description: strings.Repeat("x", FallbackMaxBodyBytes), InputSchema: []byte(`{}`)}}
	if _, err := BuildFallbackRequest(request, "instruction"); err == nil {
		t.Fatal("accepted oversized model request")
	}
}

func TestFallbackDoesNotRedirect(t *testing.T) {
	var calls int
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer destination.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()

	endpoint, err := ParseModelProxyURL(upstream.URL, true)
	if err != nil {
		t.Fatal(err)
	}

	req, err := NewFallbackRequest(t.Context(), endpoint, []byte(`{}`), "license", "fingerprint", Descriptor{Header: descriptorCommandHeader, Value: "npx package"})
	if err != nil {
		t.Fatal(err)
	}

	response, err := NewFallbackHTTPClient().Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusTemporaryRedirect || calls != 0 {
		t.Fatal("followed redirect")
	}
}

func TestDeploymentDescriptors(t *testing.T) {
	tests := []struct {
		name     string
		manifest types.MCPServerManifest
		resolved mcp.ServerConfig
		header   string
		value    string
	}{
		{
			name:     "remote origin only",
			manifest: types.MCPServerManifest{Runtime: types.RuntimeRemote, RemoteConfig: &types.RemoteRuntimeConfig{URL: "https://unused.example"}},
			resolved: mcp.ServerConfig{URL: "https://user:password@EXAMPLE.com:8443/tenant/token?secret=yes#fragment"},
			header:   descriptorURLHeader,
			value:    "https://example.com:8443",
		},
		{
			name:     "npx in container",
			manifest: types.MCPServerManifest{Runtime: types.RuntimeNPX, NPXConfig: &types.NPXRuntimeConfig{Package: "@org/server@1.0", Args: []string{"--secret=password"}}},
			resolved: mcp.ServerConfig{ContainerImage: "wrapper:latest", URL: "http://wrapper"},
			header:   descriptorCommandHeader,
			value:    "npx @org/server@1.0",
		},
		{
			name:     "uvx URL package",
			manifest: types.MCPServerManifest{Runtime: types.RuntimeUVX, UVXConfig: &types.UVXRuntimeConfig{Package: "https://user:secret@example.com/private.whl?token=secret"}},
			header:   descriptorCommandHeader,
			value:    "uvx https://example.com",
		},
		{
			name:     "uvx package with version",
			manifest: types.MCPServerManifest{Runtime: types.RuntimeUVX, UVXConfig: &types.UVXRuntimeConfig{Package: "example-server==1.2.3", Command: "custom-executable", Args: []string{"--secret=password"}}},
			header:   descriptorCommandHeader,
			value:    "uvx example-server==1.2.3",
		},
		{
			name:     "npx URL package",
			manifest: types.MCPServerManifest{Runtime: types.RuntimeNPX, NPXConfig: &types.NPXRuntimeConfig{Package: "https://user:secret@example.com/package.tgz?token=secret"}},
			header:   descriptorCommandHeader,
			value:    "npx https://example.com",
		},
		{
			name:     "container digest",
			manifest: types.MCPServerManifest{Runtime: types.RuntimeContainerized, ContainerizedConfig: &types.ContainerizedRuntimeConfig{Image: "registry.example:5000/org/server:v1@sha256:" + strings.Repeat("a", 64)}},
			header:   descriptorImageHeader,
			value:    "registry.example:5000/org/server:v1@sha256:" + strings.Repeat("a", 64),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DeploymentDescriptor(tt.manifest, tt.resolved)
			if err != nil {
				t.Fatal(err)
			}

			if got.Header != tt.header || got.Value != tt.value {
				t.Fatalf("descriptor = %#v", got)
			}
		})
	}

	for _, value := range []string{"", "package --secret", "package\nsecret", "package,other", strings.Repeat("a", 2045), strings.Repeat("a", 2049)} {
		if _, err := DeploymentDescriptor(types.MCPServerManifest{Runtime: types.RuntimeNPX, NPXConfig: &types.NPXRuntimeConfig{Package: value}}, mcp.ServerConfig{}); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
