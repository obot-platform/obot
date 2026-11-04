package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	gatewayConfigCredentialContext = "llm-core"
)

type gatewayConfigEnvelope struct {
	types.Credential
	// Credential.Encrypted is stored by GORM but excluded from JSON.
	// Resource payloads must serialize the flag for the credential decryption helpers.
	Encrypted bool `json:"encrypted"`
}

// SealGatewayConfig uses the same encryption policy and associated data as Obot credentials.
// It does not write a credential row: the caller persists the envelope in its transaction.
func (c *Client) SealGatewayConfig(ctx context.Context, id string, data []byte) ([]byte, error) {
	credential := types.Credential{Context: gatewayConfigCredentialContext, Name: id, Secrets: map[string]string{"config": string(data)}}
	if err := c.encryptCredential(ctx, &credential); err != nil {
		return nil, err
	}
	return json.Marshal(gatewayConfigEnvelope{Credential: credential, Encrypted: credential.Encrypted})
}

// OpenGatewayConfig accepts plaintext envelopes from before encryption was enabled.
// Persisted changes use the current encryption policy; reads do not rewrite data.
func (c *Client) OpenGatewayConfig(ctx context.Context, id string, data []byte) ([]byte, error) {
	var envelope gatewayConfigEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}

	credential := envelope.Credential
	credential.Encrypted = envelope.Encrypted
	if credential.Context != gatewayConfigCredentialContext || credential.Name != id {
		return nil, fmt.Errorf("gateway configuration identity mismatch")
	}
	if credential.Encrypted && (len(credential.Secrets) != 1 || credential.Secrets[credentialEncryptedSecretsKey] == "") {
		return nil, fmt.Errorf("malformed encrypted gateway configuration")
	}
	if credential.Encrypted && (c.encryptionConfig == nil || c.encryptionConfig.Transformers[credentialGroupResource] == nil) {
		return nil, fmt.Errorf("gateway configuration encryption provider unavailable")
	}

	if err := c.decryptCredential(ctx, &credential); err != nil {
		return nil, err
	}

	raw, ok := credential.Secrets["config"]
	if !ok {
		return nil, fmt.Errorf("gateway configuration document missing")
	}
	return []byte(raw), nil
}
