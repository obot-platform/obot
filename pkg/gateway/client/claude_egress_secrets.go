package client

import (
	"context"
	"fmt"
	"slices"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func (c *Client) storeClaudeEgressSecrets(ctx context.Context, environment *types.ClaudeEnvironment) (*types.ClaudeEnvironment, error) {
	stored := *environment
	stored.EgressSecrets = slices.Clone(environment.EgressSecrets)
	stored.Profiles = slices.Clone(environment.Profiles)
	values := map[string]string{}
	collect := func(secrets []types.ClaudeEgressSecret) {
		for i := range secrets {
			values[secrets[i].ID] = secrets[i].Value
			secrets[i].Value = ""
		}
	}
	collect(stored.EgressSecrets)
	for i := range stored.Profiles {
		stored.Profiles[i].EgressSecrets = slices.Clone(stored.Profiles[i].EgressSecrets)
		collect(stored.Profiles[i].EgressSecrets)
	}
	credential := types.Credential{Context: "claude-egress", Name: environment.ID, Secrets: values}
	if len(values) > 0 {
		if err := c.encryptCredential(ctx, &credential); err != nil {
			return nil, err
		}
	}
	stored.EgressSecretValues = credential.Secrets
	stored.EgressSecretsEncrypted = credential.Encrypted
	return &stored, nil
}

func (c *Client) revealClaudeEgressSecrets(ctx context.Context, environment *types.ClaudeEnvironment) error {
	credential := types.Credential{Context: "claude-egress", Name: environment.ID, Secrets: environment.EgressSecretValues, Encrypted: environment.EgressSecretsEncrypted}
	if credential.Encrypted && (c.encryptionConfig == nil || c.encryptionConfig.Transformers[credentialGroupResource] == nil) {
		return fmt.Errorf("claude egress secrets encryption provider is unavailable")
	}
	if err := c.decryptCredential(ctx, &credential); err != nil {
		return err
	}
	reveal := func(secrets []types.ClaudeEgressSecret) error {
		for i := range secrets {
			secret, ok := credential.Secrets[secrets[i].ID]
			if !ok {
				return fmt.Errorf("claude egress secret value is missing")
			}
			secrets[i].Value = secret
		}
		return nil
	}
	if err := reveal(environment.EgressSecrets); err != nil {
		return err
	}
	for i := range environment.Profiles {
		if err := reveal(environment.Profiles[i].EgressSecrets); err != nil {
			return err
		}
	}
	return nil
}
