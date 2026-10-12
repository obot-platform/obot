package client

import (
	"encoding/json"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

func TestClaudeEgressSecretsStorage(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{credentialGroupResource: testTransformer{}}}
	environment := &types.ClaudeEnvironment{
		ID: "env", EnvironmentID: "ccpool_env",
		EgressSecrets: []types.ClaudeEgressSecret{{ID: "ces-env", Hostname: "api.example.com", Header: "authorization", Value: "Bearer env-secret"}},
		Profiles:      []types.ClaudeEnvironmentProfile{{Name: "profile", EgressSecrets: []types.ClaudeEgressSecret{{ID: "ces-profile", Hostname: "api.example.com", Header: "authorization", Value: "Bearer profile-secret"}}}},
	}
	require.NoError(t, c.SaveClaudeEnvironment(t.Context(), environment, nil))
	require.Equal(t, "Bearer env-secret", environment.EgressSecrets[0].Value)
	var stored types.ClaudeEnvironment
	require.NoError(t, c.db.WithContext(t.Context()).First(&stored, "id = ?", environment.ID).Error)
	require.True(t, stored.EgressSecretsEncrypted)
	require.Empty(t, stored.EgressSecrets[0].Value)
	require.Empty(t, stored.Profiles[0].EgressSecrets[0].Value)
	raw, err := json.Marshal(stored.EgressSecretValues)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "env-secret")
	require.NotContains(t, string(raw), "profile-secret")
	revealed, err := c.ClaudeEnvironment(t.Context(), environment.ID)
	require.NoError(t, err)
	require.Equal(t, environment.EgressSecrets, revealed.EgressSecrets)
	require.Equal(t, environment.Profiles, revealed.Profiles)
	require.NoError(t, c.EnsureClaudeTagUser(t.Context(), revealed))
	require.NoError(t, c.db.WithContext(t.Context()).First(&stored, "id = ?", environment.ID).Error)
	require.Empty(t, stored.EgressSecrets[0].Value, "service-user creation must not write decrypted metadata")
	require.True(t, stored.EgressSecretsEncrypted)
	require.NoError(t, c.SaveClaudeEnvironment(t.Context(), revealed, nil))
	again, err := c.ClaudeEnvironment(t.Context(), environment.ID)
	require.NoError(t, err)
	require.Equal(t, environment.EgressSecrets, again.EgressSecrets)
	c.encryptionConfig = nil
	_, err = c.ClaudeEnvironment(t.Context(), environment.ID)
	require.ErrorContains(t, err, "encryption provider")
}

func TestClaudeEgressSecretsWithoutEncryption(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = nil
	environment := &types.ClaudeEnvironment{
		ID: "plain", EnvironmentID: "ccpool_plain",
		EgressSecrets: []types.ClaudeEgressSecret{{ID: "ces-plain", Hostname: "api.example.com", Header: "authorization", Value: "plain-secret"}},
	}
	require.NoError(t, c.SaveClaudeEnvironment(t.Context(), environment, nil))
	var stored types.ClaudeEnvironment
	require.NoError(t, c.db.WithContext(t.Context()).First(&stored, "id = ?", environment.ID).Error)
	require.False(t, stored.EgressSecretsEncrypted)
	require.Equal(t, "plain-secret", stored.EgressSecretValues["ces-plain"])
	revealed, err := c.ClaudeEnvironment(t.Context(), environment.ID)
	require.NoError(t, err)
	require.Equal(t, environment.EgressSecrets, revealed.EgressSecrets)
}
