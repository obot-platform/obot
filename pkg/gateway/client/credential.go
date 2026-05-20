package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

var credentialGroupResource = schema.GroupResource{
	Group:    "obot.obot.ai",
	Resource: "credentials",
}

const credentialEncryptedSecretsKey = "_obot_encrypted_env"

type ListCredentialsOptions struct {
	CredentialContexts []string
	AllContexts        bool
}

// ListCredentials returns the credentials in the given context.
// If AllContexts is true, CredentialContexts is ignored and credentials from all contexts are returned.
// The secrets in the returned credentials are blanked out for security; use RevealCredential to get the secrets for a specific credential.
func (c *Client) ListCredentials(ctx context.Context, opts ListCredentialsOptions) ([]types.Credential, error) {
	var credentials []types.Credential
	if len(opts.CredentialContexts) == 0 && !opts.AllContexts {
		return credentials, nil
	}

	db := c.db.WithContext(ctx)
	if !opts.AllContexts {
		db = db.Where("context IN ?", opts.CredentialContexts)
	}

	if err := db.Find(&credentials).Error; err != nil {
		return nil, fmt.Errorf("failed to list credentials: %w", err)
	}

	for i := range credentials {
		if err := c.decryptCredential(ctx, &credentials[i]); err != nil {
			return nil, fmt.Errorf("failed to decrypt credential: %w", err)
		}
		credentials[i].Secrets = blankCredentialSecrets(credentials[i].Secrets)
	}

	return credentials, nil
}

type CredentialNotFoundError struct {
	Contexts []string
	Name     string
}

func (e CredentialNotFoundError) Unwrap() error {
	// This allows errors.Is(err, gorm.ErrRecordNotFound) to work for CredentialNotFoundError.
	return gorm.ErrRecordNotFound
}

func (e CredentialNotFoundError) Error() string {
	return fmt.Sprintf("credential not found: contexts=%v, name=%s", e.Contexts, e.Name)
}

// RevealCredential returns the first credential matching name in the ordered list of contexts.
func (c *Client) RevealCredential(ctx context.Context, contexts []string, name string) (types.Credential, error) {
	var credential types.Credential
	if len(contexts) == 0 {
		return credential, CredentialNotFoundError{Contexts: contexts, Name: name}
	}

	for _, credentialContext := range contexts {
		if err := c.db.WithContext(ctx).Where("context = ? AND name = ?", credentialContext, name).First(&credential).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return credential, err
		}
		if err := c.decryptCredential(ctx, &credential); err != nil {
			return credential, fmt.Errorf("failed to decrypt credential: %w", err)
		}
		return credential, nil
	}

	return credential, CredentialNotFoundError{Contexts: contexts, Name: name}
}

// UpsertCredential creates or replaces a credential identified by context+name.
func (c *Client) UpsertCredential(ctx context.Context, credential types.Credential) error {
	if credential.Context == "" || credential.Name == "" {
		return fmt.Errorf("credential context and name are required")
	}
	if credential.Secrets == nil {
		credential.Secrets = map[string]string{}
	}
	credential.Encrypted = false
	if err := c.encryptCredential(ctx, &credential); err != nil {
		return fmt.Errorf("failed to encrypt credential: %w", err)
	}

	return c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "context"}, {Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{"secrets", "encrypted"}),
	}).Create(&credential).Error
}

// DeleteCredential deletes a credential if it exists and returns whether a credential was deleted.
func (c *Client) DeleteCredential(ctx context.Context, context, name string) (bool, error) {
	result := c.db.WithContext(ctx).Where("context = ? AND name = ?", context, name).Delete(&types.Credential{})
	if result.Error != nil {
		return false, fmt.Errorf("failed to delete credential: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (c *Client) encryptCredential(ctx context.Context, credential *types.Credential) error {
	if c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[credentialGroupResource]
	if transformer == nil {
		return nil
	}

	secretsJSON, err := json.Marshal(credential.Secrets)
	if err != nil {
		return fmt.Errorf("failed to marshal secrets: %w", err)
	}

	b, err := transformer.TransformToStorage(ctx, secretsJSON, credentialDataCtx(credential))
	if err != nil {
		return err
	}

	credential.Secrets = map[string]string{
		credentialEncryptedSecretsKey: base64.StdEncoding.EncodeToString(b),
	}
	credential.Encrypted = true
	return nil
}

func (c *Client) decryptCredential(ctx context.Context, credential *types.Credential) error {
	if !credential.Encrypted || len(credential.Secrets) != 1 || c.encryptionConfig == nil {
		return nil
	}

	transformer := c.encryptionConfig.Transformers[credentialGroupResource]
	if transformer == nil {
		return nil
	}

	encryptedSecrets := credential.Secrets[credentialEncryptedSecretsKey]
	if encryptedSecrets == "" {
		return fmt.Errorf("encrypted secrets is missing")
	}

	decoded, err := base64.StdEncoding.DecodeString(encryptedSecrets)
	if err != nil {
		return fmt.Errorf("failed to decode encrypted secrets: %w", err)
	}

	out, _, err := transformer.TransformFromStorage(ctx, decoded, credentialDataCtx(credential))
	if err != nil {
		return err
	}

	var secrets map[string]string
	if err := json.Unmarshal(out, &secrets); err != nil {
		return fmt.Errorf("failed to unmarshal secrets: %w", err)
	}

	credential.Secrets = secrets
	return nil
}

func credentialDataCtx(credential *types.Credential) value.Context {
	return value.DefaultContext(fmt.Sprintf("%s///%s", credential.Name, credential.Context))
}

func blankCredentialSecrets(secrets map[string]string) map[string]string {
	if len(secrets) == 0 {
		return secrets
	}
	blank := make(map[string]string, len(secrets))
	for key := range secrets {
		blank[key] = ""
	}
	return blank
}
