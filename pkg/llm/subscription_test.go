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
		codex   bool
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
			name:    "Codex subscription",
			headers: http.Header{"Authorization": {"Bearer eyJfixture.payload.signature"}, "User-Agent": {"codex_cli_rs/0.167.0"}},
			codex:   true,
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
			require.Equal(t, tc.codex, IsCodexSubscription(tc.headers))
		})
	}
}
