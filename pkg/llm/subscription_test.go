package llm

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSubscriptionCredentialModes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		headers http.Header
		claude  bool
		chatgpt bool
	}{
		{
			name: "configured credentials",
		},
		{
			name:    "Claude OAuth prefix without CLI identity",
			headers: http.Header{"Authorization": {"bEaReR Sk-AnT-OaT-fixture"}},
			claude:  true,
		},
		{
			name:    "Claude OAuth prefix with unsupported CLI identity",
			headers: http.Header{"Authorization": {"Bearer sk-ant-oat-fixture"}, "User-Agent": {"curl/1"}},
			claude:  true,
		},
		{
			name:    "ChatGPT subscription",
			headers: http.Header{"Authorization": {"Bearer eyJfixture.payload.signature"}, "User-Agent": {"codex_cli_rs/0.167.0"}},
			chatgpt: true,
		},
		{
			name:    "Codex API key",
			headers: http.Header{"Authorization": {"Bearer sk-fixture"}, "User-Agent": {"codex_cli_rs/0.167.0"}},
		},
		{
			name:    "JWT without Codex identity",
			headers: http.Header{"Authorization": {"Bearer eyJfixture.payload.signature"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.claude, IsClaudeSubscription(tc.headers))
			require.Equal(t, tc.chatgpt, IsChatGPTSubscription(tc.headers))
		})
	}
}

func TestStripProviderCredentials(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider string
		auth     string
		enabled  bool
		keepAuth bool
		keepID   bool
	}{
		{
			name:     "Claude subscription",
			provider: "anthropic",
			auth:     "Bearer sk-ant-oat-fixture",
			enabled:  true,
			keepAuth: true,
		},
		{
			name:     "Claude credential on OpenAI",
			provider: "openai",
			auth:     "Bearer sk-ant-oat-fixture",
			enabled:  true,
		},
		{
			name:     "ChatGPT subscription",
			provider: "openai",
			auth:     "Bearer eyJfixture.payload.signature",
			enabled:  true,
			keepAuth: true,
			keepID:   true,
		},
		{
			name:     "ChatGPT credential on Anthropic",
			provider: "anthropic",
			auth:     "Bearer eyJfixture.payload.signature",
			enabled:  true,
		},
		{
			name:     "disabled passthrough",
			provider: "openai",
			auth:     "Bearer eyJfixture.payload.signature",
		},
		{
			name:     "configured credential",
			provider: "openai",
			auth:     "Bearer sk-fixture",
			enabled:  true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{
				"Authorization":       {tc.auth},
				"User-Agent":          {"codex_cli_rs/0.167.0"},
				"Chatgpt-Account-Id":  {"fixture-account"},
				"X-Api-Key":           {"fixture-key"},
				"Api-Key":             {"fixture-key"},
				"X-Goog-Api-Key":      {"fixture-key"},
				"Cookie":              {"fixture-cookie"},
				"Openai-Organization": {"fixture-organization"},
				"Openai-Project":      {"fixture-project"},
			}
			require.Equal(t, tc.keepAuth, StripProviderCredentials(headers, tc.provider, tc.enabled))
			require.Equal(t, tc.keepAuth, headers.Get("Authorization") != "")
			require.Equal(t, tc.keepID, headers.Get("ChatGPT-Account-ID") != "")
			for _, name := range []string{"X-Api-Key", "Api-Key", "X-Goog-Api-Key", "Cookie", "OpenAI-Organization", "OpenAI-Project"} {
				require.Empty(t, headers.Get(name), name)
			}
		})
	}
}
