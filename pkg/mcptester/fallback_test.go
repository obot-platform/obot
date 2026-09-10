package mcptester

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
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

	req, err := NewFallbackRequest(t.Context(), endpoint, body, "signed==.KEY", "machine-id")
	if err != nil {
		t.Fatal(err)
	}

	if req.Header.Get("Authorization") != "Bearer signed==.KEY" || req.Header.Get("X-Obot-Machine-Fingerprint") != "machine-id" || len(req.Header) != 5 || req.GetBody != nil {
		t.Fatalf("unexpected request: %#v", req)
	}

	if _, err := NewFallbackRequest(t.Context(), endpoint, body, "", "machine-id"); err == nil {
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

	req, err := NewFallbackRequest(t.Context(), endpoint, []byte(`{}`), "license", "fingerprint")
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
