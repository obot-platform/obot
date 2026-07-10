package client

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	deviceEnrollSecretLength = 32 // 32 bytes = 256 bits of entropy
	deviceEnrollPrefix       = "ode1"
	// deviceEnrollCredentialFormat lays out an enrollment credential
	// (ode1-<configuration_id>-<key_id>-<secret>) for both minting and parsing.
	// The width in %4s must equal len(deviceEnrollPrefix): Sscanf reads exactly
	// that many characters, while Sprintf treats it as a minimum and would not
	// flag a longer prefix.
	deviceEnrollCredentialFormat = "%4s-%d-%d-%s"
)

// CreateMDMConfiguration creates a configuration and mints its first enrollment
// key. The plaintext credential is returned exactly once.
func (c *Client) CreateMDMConfiguration(ctx context.Context, createdBy uint, name, description string) (*types.MDMConfiguration, *types.DeviceEnrollmentKeyCreateResponse, error) {
	var (
		configuration types.MDMConfiguration
		key           *types.DeviceEnrollmentKeyCreateResponse
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		configuration = types.MDMConfiguration{
			Name:        name,
			Description: description,
			CreatedBy:   createdBy,
			CreatedAt:   time.Now(),
		}
		if err := tx.Create(&configuration).Error; err != nil {
			return fmt.Errorf("failed to create MDM configuration: %w", err)
		}
		var err error
		key, err = createDeviceEnrollmentKey(tx, configuration.ID, createdBy, "", nil)
		return err
	}); err != nil {
		return nil, nil, err
	}
	return &configuration, key, nil
}

// GetMDMConfiguration retrieves a single configuration by ID.
func (c *Client) GetMDMConfiguration(ctx context.Context, id uint) (*types.MDMConfiguration, error) {
	var configuration types.MDMConfiguration
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&configuration).Error; err != nil {
		return nil, err
	}
	return &configuration, nil
}

// ListMDMConfigurations returns all configurations, newest first.
func (c *Client) ListMDMConfigurations(ctx context.Context) ([]types.MDMConfiguration, error) {
	var configurations []types.MDMConfiguration
	if err := c.db.WithContext(ctx).Order("created_at DESC").Find(&configurations).Error; err != nil {
		return nil, fmt.Errorf("failed to list MDM configurations: %w", err)
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

	key := types.DeviceEnrollmentKey{
		MDMConfigurationID: configurationID,
		Name:               name,
		HashedSecret:       string(hashedSecret),
		CreatedBy:          createdBy,
		CreatedAt:          time.Now(),
		ExpiresAt:          expiresAt,
	}
	if err := tx.Create(&key).Error; err != nil {
		return nil, fmt.Errorf("failed to create device enrollment key: %w", err)
	}

	credential := fmt.Sprintf(deviceEnrollCredentialFormat, deviceEnrollPrefix, configurationID, key.ID, secret)
	return &types.DeviceEnrollmentKeyCreateResponse{
		DeviceEnrollmentKey:  key,
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
		_ = c.db.WithContext(ctx).Model(&types.DeviceEnrollmentKey{}).Where("id = ?", key.ID).Update("last_used_at", now).Error
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
	if prefix != deviceEnrollPrefix {
		return 0, 0, "", fmt.Errorf("invalid device enrollment credential prefix")
	}
	return configurationID, keyID, secret, nil
}
