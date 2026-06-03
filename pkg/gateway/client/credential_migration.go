package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

const (
	gptscriptCredentialsMigrationName           = "gptscript_credentials_to_gateway_credentials"
	toolReferenceCredentialContextMigrationName = "toolreference_credential_context_to_name"
)

type gptscriptCredentialSecret struct {
	Env map[string]string `json:"env"`
}

type gptscriptCredential struct {
	ID        uint `gorm:"primary_key"`
	CreatedAt time.Time
	ServerURL string `gorm:"unique"`
	Username  string
	Secret    string
}

// MigrateGPTScriptCredentials migrates existing GPTScript credentials into the
// gateway credentials table. The old GPTScript id, server_url, and username
// fields are intentionally not copied; server_url only supplies the new name.
func (c *Client) MigrateGPTScriptCredentials(ctx context.Context, oldDB *gorm.DB) error {
	return c.migrateIfNotRun(ctx, gptscriptCredentialsMigrationName, func(_ *gorm.DB) error {
		if oldDB == nil || !oldDB.Migrator().HasTable(gptscriptCredential{}) {
			return nil
		}

		var oldCredentials []gptscriptCredential
		if err := oldDB.WithContext(ctx).Find(&oldCredentials).Error; err != nil {
			return fmt.Errorf("failed to list GPTScript credentials: %w", err)
		}

		for _, oldCredential := range oldCredentials {
			name, context, _ := strings.Cut(oldCredential.ServerURL, "///")
			if name == "" || context == "" {
				continue
			}

			env, err := c.gptscriptCredentialEnv(ctx, context, name, oldCredential.Secret)
			if err != nil {
				return fmt.Errorf("failed to decode GPTScript credential %q in context %q: %w", name, context, err)
			}

			if err := c.UpsertCredential(ctx, gatewaytypes.Credential{
				Context: context,
				Name:    name,
				Secrets: env,
			}); err != nil {
				return fmt.Errorf("failed to migrate GPTScript credential %q in context %q: %w", name, context, err)
			}
		}

		if err := oldDB.Migrator().DropTable(gptscriptCredential{}); err != nil {
			return fmt.Errorf("failed to drop old GPTScript credentials table: %w", err)
		}

		return nil
	})
}

// MigrateToolReferenceCredentialContexts moves credentials that were scoped by
// a ToolReference UID into a context matching their credential name. It must run
// after MigrateGPTScriptCredentials because that migration creates the gateway
// credential rows from the old GPTScript credential store.
func (c *Client) MigrateToolReferenceCredentialContexts(ctx context.Context) error {
	return c.migrateIfNotRun(ctx, toolReferenceCredentialContextMigrationName, func(tx *gorm.DB) error {
		if !tx.Migrator().HasTable("toolreference") {
			return nil
		}

		var uids []string
		if err := tx.Table("toolreference").Distinct("uid").Where("uid <> ?", "").Pluck("uid", &uids).Error; err != nil {
			return fmt.Errorf("failed to list ToolReference UIDs: %w", err)
		}

		for _, uid := range uids {
			var credentials []gatewaytypes.Credential
			if err := tx.Where("context = ?", uid).Find(&credentials).Error; err != nil {
				return fmt.Errorf("failed to list credentials for ToolReference UID %q: %w", uid, err)
			}

			for _, credential := range credentials {
				if credential.Context == credential.Name {
					continue
				}

				oldContext := credential.Context
				if err := c.moveCredentialToNameContext(ctx, &credential); err != nil {
					return fmt.Errorf("failed to migrate credential %q from context %q: %w", credential.Name, oldContext, err)
				}

				if err := tx.Model(&credential).Select("Context", "Secrets", "Encrypted").Updates(credential).Error; err != nil {
					return fmt.Errorf("failed to update credential %q from context %q to %q: %w", credential.Name, oldContext, credential.Context, err)
				}
			}
		}

		return tx.Migrator().DropTable("toolreference")
	})
}

func (c *Client) moveCredentialToNameContext(ctx context.Context, credential *gatewaytypes.Credential) error {
	if credential.Encrypted {
		if c.encryptionConfig == nil || c.encryptionConfig.Transformers[credentialGroupResource] == nil {
			return fmt.Errorf("credential is encrypted but encryption is not configured")
		}
	}

	if err := c.decryptCredential(ctx, credential); err != nil {
		return fmt.Errorf("failed to decrypt credential: %w", err)
	}

	credential.Context = credential.Name
	credential.Encrypted = false
	if credential.Secrets == nil {
		credential.Secrets = map[string]string{}
	}

	if err := c.encryptCredential(ctx, credential); err != nil {
		return fmt.Errorf("failed to encrypt credential: %w", err)
	}

	return nil
}

func (c *Client) migrateIfNotRun(ctx context.Context, name string, f func(*gorm.DB) error) error {
	db := c.db.WithContext(ctx)

	var migration gatewaytypes.Migration
	if err := db.Where("name = ?", name).First(&migration).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		if err := f(tx); err != nil {
			return err
		}

		return tx.Create(&gatewaytypes.Migration{Name: name}).Error
	})
}

func (c *Client) gptscriptCredentialEnv(ctx context.Context, credentialContext, name, secret string) (map[string]string, error) {
	var secretMap map[string]json.RawMessage
	if err := json.Unmarshal([]byte(secret), &secretMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal secret: %w", err)
	}

	secretBytes := []byte(secret)
	if len(secretMap) == 1 {
		if encryptedSecret, ok := secretMap["e"]; ok {
			if c.encryptionConfig == nil {
				return nil, fmt.Errorf("secret is encrypted but encryption is not configured")
			}

			transformer := c.encryptionConfig.Transformers[credentialGroupResource]
			if transformer == nil {
				return nil, fmt.Errorf("secret is encrypted but no credential transformer is configured")
			}

			var encoded string
			if err := json.Unmarshal(encryptedSecret, &encoded); err != nil {
				return nil, fmt.Errorf("failed to unmarshal encrypted secret: %w", err)
			}

			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err != nil {
				return nil, fmt.Errorf("failed to decode encrypted secret: %w", err)
			}

			out, _, err := transformer.TransformFromStorage(ctx, decoded, credentialDataCtx(&gatewaytypes.Credential{
				Context: credentialContext,
				Name:    name,
			}))
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt secret: %w", err)
			}
			secretBytes = out
		}
	}

	var credentialSecret gptscriptCredentialSecret
	if err := json.Unmarshal(secretBytes, &credentialSecret); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credential env: %w", err)
	}
	if credentialSecret.Env == nil {
		credentialSecret.Env = map[string]string{}
	}

	return credentialSecret.Env, nil
}
