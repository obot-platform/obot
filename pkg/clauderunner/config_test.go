package clauderunner

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestManagedSettingsValidation(t *testing.T) {
	for _, test := range []struct {
		name  string
		input string
		valid bool
	}{
		{
			name:  "unset",
			valid: true,
		},
		{
			name:  "empty object",
			input: "{}",
			valid: true,
		},
		{
			name:  "advanced unknown keys",
			input: `{"customFutureSetting":true,"permissions":{"deny":["Read(./secret)"]}}`,
			valid: true,
		},
		{
			name:  "array",
			input: "[]",
		},
		{
			name:  "null",
			input: "null",
		},
		{
			name:  "scalar",
			input: "42",
		},
		{
			name:  "malformed",
			input: "{",
		},
		{
			name:  "oversized",
			input: `{"text":"` + strings.Repeat("x", 65536) + `"}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := types.ClaudeRuntime{
				ManagedSettings: json.RawMessage(test.input),
			}
			DefaultRuntime(&runtime)
			runtime = WithDefaultImages(runtime, "worker:main", "runner@sha256:"+strings.Repeat("a", 64))

			err := ValidateRuntime(runtime)
			if test.valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "managed settings")
			}
		})
	}
}

func TestEgressWildcardValidation(t *testing.T) {
	for _, tc := range []struct {
		host  string
		valid bool
	}{
		{host: "github.com", valid: true},
		{host: "*.github.com", valid: true},
		{host: "*.api.github.com", valid: true},
		{host: "*"},
		{host: "*github.com"},
		{host: "github.*"},
		{host: "*.*.github.com"},
		{host: "*.GitHub.com"},
		{host: "*.github.com."},
		{host: "*.github..com"},
		{host: "https://*.github.com"},
		{host: "*.github.com:443"},
		{host: "*.127.0.0.1"},
		{host: "*.-github.com"},
		{host: "*." + strings.Repeat("a", 64) + ".com"},
	} {
		t.Run(tc.host, func(t *testing.T) {
			runtime := types.ClaudeRuntime{EgressHosts: []string{tc.host}}
			DefaultRuntime(&runtime)
			runtime = WithDefaultImages(runtime, "worker:main", "runner@sha256:"+strings.Repeat("a", 64))
			err := ValidateRuntime(runtime)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
