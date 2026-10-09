package client

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
)

func TestSaveClaudeEnvironmentWithoutEncryption(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = nil
	environment := &types.ClaudeEnvironment{
		ID:            "test-environment",
		DisplayName:   "Test environment",
		EnvironmentID: "ccpool_test",
	}
	secrets := map[string]string{"key": "test-key", "token": "test-token"}

	require.NoError(t, c.SaveClaudeEnvironment(t.Context(), environment, secrets))
	credential, err := c.RevealCredential(t.Context(), []string{"claude-environment"}, environment.ID)
	require.NoError(t, err)
	require.False(t, credential.Encrypted)
	require.Equal(t, secrets, credential.Secrets)

	environment.DisplayName = "Renamed"
	require.NoError(t, c.SaveClaudeEnvironment(t.Context(), environment, nil))
	saved, err := c.ClaudeEnvironment(t.Context(), environment.ID)
	require.NoError(t, err)
	require.Equal(t, "Renamed", saved.DisplayName)
	credential, err = c.RevealCredential(t.Context(), []string{"claude-environment"}, environment.ID)
	require.NoError(t, err)
	require.Equal(t, secrets, credential.Secrets)
}
