package client

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	deviceEnrollSecretLength = 32 // 32 bytes = 256 bits of entropy
	// deviceEnrollCredentialFormat lays out an enrollment credential
	// (ode1-<configuration_id>-<key_id>-<secret>) for both minting and parsing.
	// The width in %4s must equal len(system.DeviceEnrollmentPrefix): Sscanf reads exactly
	// that many characters, while Sprintf treats it as a minimum and would not
	// flag a longer prefix.
	deviceEnrollCredentialFormat = "%4s-%d-%d-%s"
)

// CreateMDMConfiguration creates a configuration and atomically persists any
// rendered artifact content. The first configuration becomes the default.
func (c *Client) CreateMDMConfiguration(ctx context.Context, createdBy uint, configuration *types.MDMConfiguration) (*types.MDMConfiguration, error) {
	if err := normalizeMDMConfiguration(configuration); err != nil {
		return nil, err
	}
	artifacts := configuration.Artifacts
	configuration.Artifacts = nil

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		configuration.ID = 0
		configuration.CreatedBy = createdBy
		configuration.CreatedAt = time.Now().UTC()

		configuration.IsDefault = true
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(configuration)
		if result.Error != nil {
			return fmt.Errorf("failed to create MDM configuration: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			configuration.ID = 0
			configuration.IsDefault = false
			if err := tx.Create(configuration).Error; err != nil {
				return fmt.Errorf("failed to create MDM configuration: %w", err)
			}
		}
		var err error
		configuration.Artifacts, err = replaceMDMConfigurationArtifacts(tx, configuration.ID, artifacts)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return configuration, nil
}

// GetMDMConfiguration retrieves a single configuration by ID.
func (c *Client) GetMDMConfiguration(ctx context.Context, id uint) (*types.MDMConfiguration, error) {
	var configuration types.MDMConfiguration
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&configuration).Error; err != nil {
		return nil, err
	}
	if err := c.db.WithContext(ctx).Where("mdm_configuration_id = ?", id).Order("id").Find(&configuration.Artifacts).Error; err != nil {
		return nil, fmt.Errorf("failed to load MDM configuration artifacts: %w", err)
	}
	return &configuration, nil
}

// UpdateMDMConfiguration updates a configuration and atomically replaces its
// rendered artifacts in the same transaction.
func (c *Client) UpdateMDMConfiguration(ctx context.Context, configuration *types.MDMConfiguration) error {
	if configuration.ID == 0 {
		return fmt.Errorf("MDM configuration id is required")
	}
	if strings.TrimSpace(configuration.AssetDigest) == "" {
		return fmt.Errorf("MDM configuration asset digest is required")
	}
	if err := normalizeMDMConfiguration(configuration); err != nil {
		return err
	}
	artifacts := configuration.Artifacts
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Select("id").Where("id = ?", configuration.ID).First(&types.MDMConfiguration{}).Error; err != nil {
			return err
		}
		result := tx.Model(&types.MDMConfiguration{}).
			Where("id = ?", configuration.ID).
			Select("AssetDigest", "ObotSentryVersion", "Values").
			Updates(configuration)
		if result.Error != nil {
			return fmt.Errorf("failed to update MDM configuration: %w", result.Error)
		}
		_, err := replaceMDMConfigurationArtifacts(tx, configuration.ID, artifacts)
		return err
	})
}

func normalizeMDMConfiguration(configuration *types.MDMConfiguration) error {
	configuration.AssetDigest = strings.TrimSpace(configuration.AssetDigest)
	if configuration.AssetDigest == "" {
		configuration.ObotSentryVersion = ""
		configuration.Values = ""
		if len(configuration.Artifacts) != 0 {
			return fmt.Errorf("blank MDM asset digest cannot have rendered artifacts")
		}
	} else if len(configuration.Artifacts) == 0 {
		return fmt.Errorf("configured MDM asset digest requires rendered artifacts")
	}
	return nil
}

func replaceMDMConfigurationArtifacts(tx *gorm.DB, configurationID uint, artifacts []types.MDMConfigurationArtifact) ([]types.MDMConfigurationArtifact, error) {
	slugs := map[string]struct{}{}
	for i := range artifacts {
		artifact := &artifacts[i]
		artifact.Slug = strings.TrimSpace(artifact.Slug)
		artifact.Platform = strings.TrimSpace(artifact.Platform)
		artifact.OS = strings.TrimSpace(artifact.OS)
		if artifact.Slug == "" || artifact.Platform == "" || artifact.OS == "" {
			return nil, fmt.Errorf("rendered MDM artifacts require slug, platform, and OS")
		}
		if _, ok := slugs[artifact.Slug]; ok {
			return nil, fmt.Errorf("rendered MDM artifact slug %s is duplicated", artifact.Slug)
		}
		slugs[artifact.Slug] = struct{}{}
		if len(artifact.Content) == 0 {
			return nil, fmt.Errorf("rendered MDM artifact %s has empty content", artifact.Slug)
		}
		sum := sha256.Sum256(artifact.Content)
		artifact.ID = 0
		artifact.MDMConfigurationID = configurationID
		artifact.Digest = hex.EncodeToString(sum[:])
	}
	if err := tx.Where("mdm_configuration_id = ?", configurationID).Delete(&types.MDMConfigurationArtifact{}).Error; err != nil {
		return nil, fmt.Errorf("failed to replace MDM configuration artifacts: %w", err)
	}
	if len(artifacts) > 0 {
		if err := tx.Create(&artifacts).Error; err != nil {
			return nil, fmt.Errorf("failed to store MDM configuration artifacts: %w", err)
		}
	}
	return artifacts, nil
}

// InvalidateMDMConfigurationArtifacts clears rendered downloads that were
// produced from any bundle other than latestDigest. Values and AssetDigest are
// retained so an administrator can review them against the latest fields.
func (c *Client) InvalidateMDMConfigurationArtifacts(ctx context.Context, latestDigest string) error {
	// When latestDigest is empty (source removed), every configured
	// configuration's artifacts are stale, which the same inequality expresses:
	// blank configurations (asset_digest = '') have no artifacts either way.
	configurations := c.db.WithContext(ctx).Model(&types.MDMConfiguration{}).Select("id").
		Where("asset_digest <> ?", latestDigest)
	if err := c.db.WithContext(ctx).Where("mdm_configuration_id IN (?)", configurations).Delete(&types.MDMConfigurationArtifact{}).Error; err != nil {
		return fmt.Errorf("failed to invalidate MDM configuration artifacts: %w", err)
	}
	return nil
}

// ListMDMConfigurations returns the default first, then newest first.
func (c *Client) ListMDMConfigurations(ctx context.Context) ([]types.MDMConfiguration, error) {
	var configurations []types.MDMConfiguration
	if err := c.db.WithContext(ctx).Order("is_default DESC, created_at DESC").Find(&configurations).Error; err != nil {
		return nil, fmt.Errorf("failed to list MDM configurations: %w", err)
	}
	if len(configurations) == 0 {
		return configurations, nil
	}
	ids := make([]uint, 0, len(configurations))
	byID := make(map[uint]int, len(configurations))
	for i, configuration := range configurations {
		ids = append(ids, configuration.ID)
		byID[configuration.ID] = i
	}
	var artifacts []types.MDMConfigurationArtifact
	if err := c.db.WithContext(ctx).Omit("Content").Where("mdm_configuration_id IN ?", ids).Order("id").Find(&artifacts).Error; err != nil {
		return nil, fmt.Errorf("failed to list MDM configuration artifacts: %w", err)
	}
	for _, artifact := range artifacts {
		index := byID[artifact.MDMConfigurationID]
		configurations[index].Artifacts = append(configurations[index].Artifacts, artifact)
	}
	return configurations, nil
}

// DeleteMDMConfiguration removes a configuration and its enrollment keys. Devices
// enrolled into it are intentionally preserved (not deleted).
func (c *Client) DeleteMDMConfiguration(ctx context.Context, id uint) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("mdm_configuration_id = ?", id).Delete(&types.DeviceEnrollmentKey{}).Error; err != nil {
			return fmt.Errorf("failed to delete enrollment keys for configuration: %w", err)
		}
		if err := tx.Where("mdm_configuration_id = ?", id).Delete(&types.MDMConfigurationArtifact{}).Error; err != nil {
			return fmt.Errorf("failed to delete artifacts for configuration: %w", err)
		}
		if err := tx.Where("id = ?", id).Delete(&types.MDMConfiguration{}).Error; err != nil {
			return fmt.Errorf("failed to delete MDM configuration: %w", err)
		}
		return nil
	})
}

// CreateDeviceEnrollmentKey attaches an additional enrollment key to a
// configuration. Existing keys and enrolled devices are untouched.
func (c *Client) CreateDeviceEnrollmentKey(ctx context.Context, configurationID, createdBy uint, name string, expiresAt *time.Time) (*types.DeviceEnrollmentKeyCreateResponse, error) {
	var resp *types.DeviceEnrollmentKeyCreateResponse
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", configurationID).First(&types.MDMConfiguration{}).Error; err != nil {
			return err
		}
		var err error
		resp, err = createDeviceEnrollmentKey(tx, configurationID, createdBy, name, expiresAt)
		return err
	}); err != nil {
		return nil, err
	}
	return resp, nil
}

func createDeviceEnrollmentKey(tx *gorm.DB, configurationID, createdBy uint, name string, expiresAt *time.Time) (*types.DeviceEnrollmentKeyCreateResponse, error) {
	secretBytes := make([]byte, deviceEnrollSecretLength)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, fmt.Errorf("failed to generate secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)

	hashedSecret, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("failed to hash secret: %w", err)
	}

	deviceEnrollmentKey := types.DeviceEnrollmentKey{
		MDMConfigurationID: configurationID,
		Name:               name,
		HashedSecret:       string(hashedSecret),
		CreatedBy:          createdBy,
		CreatedAt:          time.Now(),
		ExpiresAt:          expiresAt,
	}
	if err := tx.Create(&deviceEnrollmentKey).Error; err != nil {
		return nil, fmt.Errorf("failed to create device enrollment key: %w", err)
	}

	credential := fmt.Sprintf(deviceEnrollCredentialFormat, system.DeviceEnrollmentPrefix, configurationID, deviceEnrollmentKey.ID, secret)
	return &types.DeviceEnrollmentKeyCreateResponse{
		DeviceEnrollmentKey:  deviceEnrollmentKey,
		EnrollmentCredential: credential,
	}, nil
}

// ListDeviceEnrollmentKeys returns the (secret-free) keys for a configuration,
// newest first.
func (c *Client) ListDeviceEnrollmentKeys(ctx context.Context, configurationID uint) ([]types.DeviceEnrollmentKey, error) {
	var keys []types.DeviceEnrollmentKey
	if err := c.db.WithContext(ctx).
		Where("mdm_configuration_id = ?", configurationID).
		Order("created_at DESC").
		Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to list device enrollment keys: %w", err)
	}
	return keys, nil
}

// DeleteDeviceEnrollmentKey removes a single enrollment key. It only stops that
// key from enrolling new devices; already-enrolled devices are unaffected.
// Scoped to the configuration so a mismatched path can't delete another
// configuration's key. Idempotent: deleting a key that doesn't exist succeeds.
func (c *Client) DeleteDeviceEnrollmentKey(ctx context.Context, configurationID, id uint) error {
	if err := c.db.WithContext(ctx).
		Where("id = ? AND mdm_configuration_id = ?", id, configurationID).
		Delete(&types.DeviceEnrollmentKey{}).Error; err != nil {
		return fmt.Errorf("failed to delete device enrollment key: %w", err)
	}
	return nil
}

// ValidateDeviceEnrollmentCredential parses an ode1-... credential, loads its
// key, and accepts it when the key exists (scoped to the configuration), is not
// expired, and the secret matches. Returns the key on success.
func (c *Client) ValidateDeviceEnrollmentCredential(ctx context.Context, credential string) (*types.DeviceEnrollmentKey, error) {
	configurationID, keyID, secret, err := ParseDeviceEnrollmentCredential(credential)
	if err != nil {
		return nil, err
	}

	var key types.DeviceEnrollmentKey
	if err := c.db.WithContext(ctx).
		Where("id = ?", keyID).
		Where("mdm_configuration_id = ?", configurationID).
		First(&key).Error; err != nil {
		return nil, err
	}

	// Check expiry before the bcrypt comparison so expired keys don't cost a
	// hash per attempt.
	if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("device enrollment credential expired")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(key.HashedSecret), []byte(secret)); err != nil {
		return nil, fmt.Errorf("invalid device enrollment credential")
	}

	// Best-effort last-used update.
	now := time.Now().UTC()
	if key.LastUsedAt == nil || now.Sub(*key.LastUsedAt) > time.Minute {
		c.db.WithContext(ctx).Model(&types.DeviceEnrollmentKey{}).Where("id = ?", key.ID).Update("last_used_at", now)
		key.LastUsedAt = &now
	}

	return &key, nil
}

// ParseDeviceEnrollmentCredential extracts the configuration ID, key ID, and secret
// from an ode1-<configuration_id>-<key_id>-<secret> credential.
func ParseDeviceEnrollmentCredential(credential string) (configurationID, keyID uint, secret string, err error) {
	var prefix string
	n, err := fmt.Sscanf(credential, deviceEnrollCredentialFormat, &prefix, &configurationID, &keyID, &secret)
	if err != nil || n != 4 {
		return 0, 0, "", fmt.Errorf("invalid device enrollment credential format")
	}
	if prefix != system.DeviceEnrollmentPrefix {
		return 0, 0, "", fmt.Errorf("invalid device enrollment credential prefix")
	}
	return configurationID, keyID, secret, nil
}
