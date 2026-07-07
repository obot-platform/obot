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
	// (ode1-<deployment_id>-<key_id>-<secret>) for both minting and parsing.
	// The width in %4s must equal len(deviceEnrollPrefix): Sscanf reads exactly
	// that many characters, while Sprintf treats it as a minimum and would not
	// flag a longer prefix.
	deviceEnrollCredentialFormat = "%4s-%d-%d-%s"
)

// CreateMDMDeployment creates a deployment and mints its first enrollment
// key. The plaintext credential is returned exactly once.
func (c *Client) CreateMDMDeployment(ctx context.Context, createdBy uint, name, description string) (*types.MDMDeployment, *types.DeviceEnrollmentKeyCreateResponse, error) {
	var (
		deployment types.MDMDeployment
		key        *types.DeviceEnrollmentKeyCreateResponse
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		deployment = types.MDMDeployment{
			Name:        name,
			Description: description,
			CreatedBy:   createdBy,
			CreatedAt:   time.Now(),
		}
		if err := tx.Create(&deployment).Error; err != nil {
			return fmt.Errorf("failed to create MDM deployment: %w", err)
		}
		var err error
		key, err = createDeviceEnrollmentKey(tx, deployment.ID, createdBy, "", nil)
		return err
	}); err != nil {
		return nil, nil, err
	}
	return &deployment, key, nil
}

// GetMDMDeployment retrieves a single deployment by ID.
func (c *Client) GetMDMDeployment(ctx context.Context, id uint) (*types.MDMDeployment, error) {
	var deployment types.MDMDeployment
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&deployment).Error; err != nil {
		return nil, err
	}
	return &deployment, nil
}

// ListMDMDeployments returns all deployments, newest first.
func (c *Client) ListMDMDeployments(ctx context.Context) ([]types.MDMDeployment, error) {
	var deployments []types.MDMDeployment
	if err := c.db.WithContext(ctx).Order("created_at DESC").Find(&deployments).Error; err != nil {
		return nil, fmt.Errorf("failed to list MDM deployments: %w", err)
	}
	return deployments, nil
}

// DeleteMDMDeployment removes a deployment and its enrollment keys. Devices
// enrolled into it are intentionally preserved (not deleted).
func (c *Client) DeleteMDMDeployment(ctx context.Context, id uint) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("mdm_deployment_id = ?", id).Delete(&types.DeviceEnrollmentKey{}).Error; err != nil {
			return fmt.Errorf("failed to delete enrollment keys for deployment: %w", err)
		}
		if err := tx.Where("id = ?", id).Delete(&types.MDMDeployment{}).Error; err != nil {
			return fmt.Errorf("failed to delete MDM deployment: %w", err)
		}
		return nil
	})
}

// CreateDeviceEnrollmentKey attaches an additional enrollment key to a
// deployment. Existing keys and enrolled devices are untouched.
func (c *Client) CreateDeviceEnrollmentKey(ctx context.Context, deploymentID, createdBy uint, name string, expiresAt *time.Time) (*types.DeviceEnrollmentKeyCreateResponse, error) {
	var resp *types.DeviceEnrollmentKeyCreateResponse
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", deploymentID).First(&types.MDMDeployment{}).Error; err != nil {
			return err
		}
		var err error
		resp, err = createDeviceEnrollmentKey(tx, deploymentID, createdBy, name, expiresAt)
		return err
	}); err != nil {
		return nil, err
	}
	return resp, nil
}

func createDeviceEnrollmentKey(tx *gorm.DB, deploymentID, createdBy uint, name string, expiresAt *time.Time) (*types.DeviceEnrollmentKeyCreateResponse, error) {
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
		MDMDeploymentID: deploymentID,
		Name:            name,
		HashedSecret:    string(hashedSecret),
		CreatedBy:       createdBy,
		CreatedAt:       time.Now(),
		ExpiresAt:       expiresAt,
	}
	if err := tx.Create(&key).Error; err != nil {
		return nil, fmt.Errorf("failed to create device enrollment key: %w", err)
	}

	credential := fmt.Sprintf(deviceEnrollCredentialFormat, deviceEnrollPrefix, deploymentID, key.ID, secret)
	return &types.DeviceEnrollmentKeyCreateResponse{
		DeviceEnrollmentKey:  key,
		EnrollmentCredential: credential,
	}, nil
}

// ListDeviceEnrollmentKeys returns the (secret-free) keys for a deployment,
// newest first.
func (c *Client) ListDeviceEnrollmentKeys(ctx context.Context, deploymentID uint) ([]types.DeviceEnrollmentKey, error) {
	var keys []types.DeviceEnrollmentKey
	if err := c.db.WithContext(ctx).
		Where("mdm_deployment_id = ?", deploymentID).
		Order("created_at DESC").
		Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to list device enrollment keys: %w", err)
	}
	return keys, nil
}

// DeleteDeviceEnrollmentKey removes a single enrollment key. It only stops that
// key from enrolling new devices; already-enrolled devices are unaffected.
// Scoped to the deployment so a mismatched path can't delete another
// deployment's key. Idempotent: deleting a key that doesn't exist succeeds.
func (c *Client) DeleteDeviceEnrollmentKey(ctx context.Context, deploymentID, id uint) error {
	if err := c.db.WithContext(ctx).
		Where("id = ? AND mdm_deployment_id = ?", id, deploymentID).
		Delete(&types.DeviceEnrollmentKey{}).Error; err != nil {
		return fmt.Errorf("failed to delete device enrollment key: %w", err)
	}
	return nil
}

// ValidateDeviceEnrollmentCredential parses an ode1-... credential, loads its
// key, and accepts it when the key exists (scoped to the deployment), is not
// expired, and the secret matches. Returns the key on success.
func (c *Client) ValidateDeviceEnrollmentCredential(ctx context.Context, credential string) (*types.DeviceEnrollmentKey, error) {
	deploymentID, keyID, secret, err := ParseDeviceEnrollmentCredential(credential)
	if err != nil {
		return nil, err
	}

	var key types.DeviceEnrollmentKey
	if err := c.db.WithContext(ctx).
		Where("id = ?", keyID).
		Where("mdm_deployment_id = ?", deploymentID).
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

// ParseDeviceEnrollmentCredential extracts the deployment ID, key ID, and secret
// from an ode1-<deployment_id>-<key_id>-<secret> credential.
func ParseDeviceEnrollmentCredential(credential string) (deploymentID, keyID uint, secret string, err error) {
	var prefix string
	n, err := fmt.Sscanf(credential, deviceEnrollCredentialFormat, &prefix, &deploymentID, &keyID, &secret)
	if err != nil || n != 4 {
		return 0, 0, "", fmt.Errorf("invalid device enrollment credential format")
	}
	if prefix != deviceEnrollPrefix {
		return 0, 0, "", fmt.Errorf("invalid device enrollment credential prefix")
	}
	return deploymentID, keyID, secret, nil
}
