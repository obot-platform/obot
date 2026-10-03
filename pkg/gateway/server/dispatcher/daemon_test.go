package dispatcher

import (
	"slices"
	"testing"
)

func TestInheritedEnv(t *testing.T) {
	tests := []struct {
		name        string
		environment map[string]string
		providerEnv map[string]string
		expected    []string
	}{
		{
			name: "copies proxy settings from Obot's environment",
			environment: map[string]string{
				"HTTP_PROXY":  "http://proxy.example.com:3128",
				"HTTPS_PROXY": "http://proxy.example.com:3128",
				"NO_PROXY":    "localhost,127.0.0.1",
			},
			providerEnv: nil,
			expected: []string{
				"HTTP_PROXY=http://proxy.example.com:3128",
				"HTTPS_PROXY=http://proxy.example.com:3128",
				"NO_PROXY=localhost,127.0.0.1",
			},
		},
		{
			name: "keeps the spelling of each variable",
			environment: map[string]string{
				"HTTP_PROXY": "http://upper.example.com:3128",
				"http_proxy": "http://lower.example.com:3128",
			},
			providerEnv: nil,
			expected: []string{
				"HTTP_PROXY=http://upper.example.com:3128",
				"http_proxy=http://lower.example.com:3128",
			},
		},
		{
			name: "copies the CA bundle a TLS-terminating proxy needs",
			environment: map[string]string{
				"SSL_CERT_FILE":      "/etc/ssl/corp.pem",
				"REQUESTS_CA_BUNDLE": "/etc/ssl/corp.pem",
			},
			providerEnv: nil,
			expected: []string{
				"SSL_CERT_FILE=/etc/ssl/corp.pem",
				"REQUESTS_CA_BUNDLE=/etc/ssl/corp.pem",
			},
		},
		{
			name: "omits variables that are not set",
			environment: map[string]string{
				"HTTP_PROXY": "http://proxy.example.com:3128",
			},
			providerEnv: nil,
			expected:    []string{"HTTP_PROXY=http://proxy.example.com:3128"},
		},
		{
			name: "never copies anything outside the allowlist",
			environment: map[string]string{
				"OBOT_SERVER_DSN": "postgres://obot:secret@localhost/obot",
				"OPENAI_API_KEY":  "sk-secret",
			},
			providerEnv: nil,
			expected:    nil,
		},
		{
			name: "skips variables the provider sets itself, in either spelling",
			environment: map[string]string{
				"HTTP_PROXY":  "http://obot.example.com:3128",
				"http_proxy":  "http://obot.example.com:3128",
				"HTTPS_PROXY": "http://obot.example.com:3128",
			},
			providerEnv: map[string]string{
				"HTTP_PROXY": "http://provider.example.com:3128",
			},
			expected: []string{"HTTPS_PROXY=http://obot.example.com:3128"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lookupEnv := func(name string) (string, bool) {
				value, ok := tt.environment[name]
				return value, ok
			}

			got := inheritedEnv(lookupEnv, tt.providerEnv)

			slices.Sort(got)
			expected := slices.Clone(tt.expected)
			slices.Sort(expected)

			if !slices.Equal(got, expected) {
				t.Fatalf("inheritedEnv() = %v, want %v", got, expected)
			}
		})
	}
}
