package handlers

import (
	"encoding/json"
	"fmt"
	"testing"

	gt "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestClaudeSecretEditing(t *testing.T) {
	old := &gt.ClaudeEnvironment{EgressSecrets: []gt.ClaudeEgressSecret{{ID: "ces-old", Hostname: "api.example.com", Header: "authorization", Value: "Bearer secret"}}}
	input := &gt.ClaudeEnvironment{EgressSecrets: []gt.ClaudeEgressSecret{{ID: "ces-old", Hostname: "api.example.com", Header: "Authorization"}}}
	require.NoError(t, prepareClaudeEgressSecrets(input, old))
	require.Equal(t, old.EgressSecrets, input.EgressSecrets)
	input.EgressSecrets[0].Hostname = "new.example.com"
	require.NoError(t, prepareClaudeEgressSecrets(input, old))
	require.NotEqual(t, "ces-old", input.EgressSecrets[0].ID)
	require.Equal(t, "Bearer secret", input.EgressSecrets[0].Value)
	input.Profiles = []gt.ClaudeEnvironmentProfile{{EgressSecrets: []gt.ClaudeEgressSecret{{Value: "profile-secret"}}}}
	redactClaudeEgressSecrets(input)
	raw, err := json.Marshal(input)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "Bearer secret")
	require.NotContains(t, string(raw), "profile-secret")
	require.NoError(t, prepareClaudeEgressSecrets(&gt.ClaudeEnvironment{}, old), "entries can be removed")
}

func TestClaudeSecretValidation(t *testing.T) {
	for _, tc := range []struct{ name, host, header, value string }{
		{
			name:   "wildcard destination",
			host:   "*.example.com",
			header: "authorization",
			value:  "secret",
		},
		{
			name:   "protected header",
			host:   "example.com",
			header: "X-Obot-Egress-Key",
			value:  "secret",
		},
		{
			name:   "audit-visible header",
			host:   "example.com",
			header: "user-agent",
			value:  "secret",
		},
		{
			name:   "invalid header",
			host:   "example.com",
			header: "bad header",
			value:  "secret",
		},
		{
			name:   "newline value",
			host:   "example.com",
			header: "authorization",
			value:  "secret\r\nHost: attacker",
		},
		{
			name:   "empty value",
			host:   "example.com",
			header: "authorization",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := &gt.ClaudeEnvironment{EgressSecrets: []gt.ClaudeEgressSecret{{Hostname: tc.host, Header: tc.header, Value: tc.value}}}
			require.Error(t, prepareClaudeEgressSecrets(input, &gt.ClaudeEnvironment{}))
		})
	}
	input := &gt.ClaudeEnvironment{EgressSecrets: []gt.ClaudeEgressSecret{{Hostname: "example.com", Header: "Authorization", Value: "secret"}}}
	require.NoError(t, prepareClaudeEgressSecrets(input, &gt.ClaudeEnvironment{}))
	require.NotEmpty(t, input.EgressSecrets[0].ID)
}

func TestClaudeEgressSecretsHeaderLimit(t *testing.T) {
	env := &gt.ClaudeEnvironment{}
	for i := range 16 {
		env.EgressSecrets = append(env.EgressSecrets, gt.ClaudeEgressSecret{
			Hostname: "api.example.com",
			Header:   fmt.Sprintf("x-secret-%d", i),
			Value:    "secret",
		})
	}
	require.NoError(t, prepareClaudeEgressSecrets(env, &gt.ClaudeEnvironment{}))
	env.Profiles = []gt.ClaudeEnvironmentProfile{{EgressSecrets: []gt.ClaudeEgressSecret{{Hostname: "api.example.com", Header: "authorization", Value: "secret"}}}}
	require.Error(t, prepareClaudeEgressSecrets(env, env))
}
