package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/storage/value"
)

const (
	credentialEncryptedSecretsKey = "_obot_encrypted_env"
)

var (
	credentialGroupResource = schema.GroupResource{
		Group:    "obot.obot.ai",
		Resource: "credentials",
	}
)

type ListCredentialsOptions struct {
	CredentialContexts []string
	AllContexts        bool
}

type CredentialNotFoundError struct {
	Contexts []string
	Name     string
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

// RevealAuthProviderCredential is RevealCredential for the credential of the auth provider namespace/name, and also
// returns the adapter type of the provider's SCIM connection, or "" when it has none. It reads both in one query, as
// it runs for every request that checks whether the provider is configured.
func (c *Client) RevealAuthProviderCredential(ctx context.Context, contexts []string, namespace, name string) (types.Credential, string, error) {
	if len(contexts) == 0 {
		return types.Credential{}, "", CredentialNotFoundError{Contexts: contexts, Name: name}
	}

	type credentialRow struct {
		types.Credential
		SCIMAdapterType *string
	}
	var rows []credentialRow
	if err := c.db.WithContext(ctx).Model(new(types.Credential)).
		Select("credentials.*, (SELECT adapter_type FROM scim_connections WHERE auth_provider_namespace = ? AND auth_provider_name = ?) AS scim_adapter_type", namespace, name).
		Where("context IN ? AND name = ?", contexts, name).
		Scan(&rows).Error; err != nil {
		return types.Credential{}, "", fmt.Errorf("failed to read the credential of auth provider %s/%s: %w", namespace, name, err)
	}

	// The first context that has the credential wins, as in RevealCredential.
	for _, credentialContext := range contexts {
		i := slices.IndexFunc(rows, func(row credentialRow) bool {
			return row.Context == credentialContext
		})
		if i < 0 {
			continue
		}

		credential := rows[i].Credential
		if err := c.decryptCredential(ctx, &credential); err != nil {
			return types.Credential{}, "", fmt.Errorf("failed to decrypt credential: %w", err)
		}
		var adapterType string
		if rows[i].SCIMAdapterType != nil {
			adapterType = *rows[i].SCIMAdapterType
		}
		return credential, adapterType, nil
	}

	return types.Credential{}, "", CredentialNotFoundError{Contexts: contexts, Name: name}
}

// HasCredential reports whether a credential with name exists in any of contexts. It does not decrypt the credential,
// so it answers even when the credential could not be decrypted.
func (c *Client) HasCredential(ctx context.Context, contexts []string, name string) (bool, error) {
	return hasCredentialTx(c.db.WithContext(ctx), contexts, name)
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

func hasCredentialTx(tx *gorm.DB, contexts []string, name string) (bool, error) {
	if len(contexts) == 0 {
		return false, nil
	}

	var count int64
	if err := tx.Model(new(types.Credential)).Where("context IN ? AND name = ?", contexts, name).Count(&count).Error; err != nil {
		return false, fmt.Errorf("failed to check for credential %s: %w", name, err)
	}
	return count > 0, nil
}
