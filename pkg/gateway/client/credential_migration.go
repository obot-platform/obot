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

const gptscriptCredentialsMigrationName = "gptscript_credentials_to_gateway_credentials"

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
	if oldDB == nil {
		return nil
	}

	var migration gatewaytypes.Migration
	if err := c.db.WithContext(ctx).Where("name = ?", gptscriptCredentialsMigrationName).First(&migration).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	} else {
		return nil
	}

	if !oldDB.Migrator().HasTable(gptscriptCredential{}) {
		return c.db.WithContext(ctx).Create(&gatewaytypes.Migration{Name: gptscriptCredentialsMigrationName}).Error
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

	return c.db.WithContext(ctx).Create(&gatewaytypes.Migration{Name: gptscriptCredentialsMigrationName}).Error
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
