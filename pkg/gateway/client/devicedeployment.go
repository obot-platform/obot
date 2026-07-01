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
)

// CreateDeviceDeployment creates a deployment and mints its first enrollment
// key. The plaintext credential is returned exactly once.
func (c *Client) CreateDeviceDeployment(ctx context.Context, createdBy uint, name, description string) (*types.DeviceDeployment, *types.DeviceEnrollmentKeyCreateResponse, error) {
	var (
		deployment types.DeviceDeployment
		key        *types.DeviceEnrollmentKeyCreateResponse
	)
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		deployment = types.DeviceDeployment{
			Name:        name,
			Description: description,
			CreatedBy:   createdBy,
			CreatedAt:   time.Now(),
		}
		if err := tx.Create(&deployment).Error; err != nil {
			return fmt.Errorf("failed to create device deployment: %w", err)
		}
		var err error
		key, err = createDeviceEnrollmentKey(tx, deployment.ID, createdBy, "", nil)
		return err
	}); err != nil {
		return nil, nil, err
	}
	return &deployment, key, nil
}

// GetDeviceDeployment retrieves a single deployment by ID.
func (c *Client) GetDeviceDeployment(ctx context.Context, id uint) (*types.DeviceDeployment, error) {
	var deployment types.DeviceDeployment
	if err := c.db.WithContext(ctx).Where("id = ?", id).First(&deployment).Error; err != nil {
		return nil, err
	}
	return &deployment, nil
}

// ListDeviceDeployments returns all deployments, newest first.
func (c *Client) ListDeviceDeployments(ctx context.Context) ([]types.DeviceDeployment, error) {
	var deployments []types.DeviceDeployment
	if err := c.db.WithContext(ctx).Order("created_at DESC").Find(&deployments).Error; err != nil {
		return nil, fmt.Errorf("failed to list device deployments: %w", err)
	}
	return deployments, nil
}

// DeleteDeviceDeployment removes a deployment and its enrollment keys. Devices
// enrolled into it are intentionally preserved (not deleted).
func (c *Client) DeleteDeviceDeployment(ctx context.Context, id uint) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("device_deployment_id = ?", id).Delete(&types.DeviceEnrollmentKey{}).Error; err != nil {
			return fmt.Errorf("failed to delete enrollment keys for deployment: %w", err)
		}
		if err := tx.Where("id = ?", id).Delete(&types.DeviceDeployment{}).Error; err != nil {
			return fmt.Errorf("failed to delete device deployment: %w", err)
		}
		return nil
	})
}

// CreateDeviceEnrollmentKey attaches an additional enrollment key to a
// deployment. Existing keys and enrolled devices are untouched.
func (c *Client) CreateDeviceEnrollmentKey(ctx context.Context, deploymentID, createdBy uint, name string, expiresAt *time.Time) (*types.DeviceEnrollmentKeyCreateResponse, error) {
	var resp *types.DeviceEnrollmentKeyCreateResponse
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", deploymentID).First(&types.DeviceDeployment{}).Error; err != nil {
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
		DeviceDeploymentID: deploymentID,
		Name:               name,
		HashedSecret:       string(hashedSecret),
		CreatedBy:          createdBy,
		CreatedAt:          time.Now(),
		ExpiresAt:          expiresAt,
	}
	if err := tx.Create(&key).Error; err != nil {
		return nil, fmt.Errorf("failed to create device enrollment key: %w", err)
	}

	// ode1-<deployment_id>-<key_id>-<secret>
	credential := fmt.Sprintf("%s-%d-%d-%s", deviceEnrollPrefix, deploymentID, key.ID, secret)
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
		Where("device_deployment_id = ?", deploymentID).
		Order("created_at DESC").
		Find(&keys).Error; err != nil {
		return nil, fmt.Errorf("failed to list device enrollment keys: %w", err)
	}
	return keys, nil
}

// DeleteDeviceEnrollmentKey removes a single enrollment key. It only stops that
// key from enrolling new devices; already-enrolled devices are unaffected.
// Scoped to the deployment so a mismatched path can't delete another
// deployment's key.
func (c *Client) DeleteDeviceEnrollmentKey(ctx context.Context, deploymentID, id uint) error {
	res := c.db.WithContext(ctx).
		Where("id = ? AND device_deployment_id = ?", id, deploymentID).
		Delete(&types.DeviceEnrollmentKey{})
	if res.Error != nil {
		return fmt.Errorf("failed to delete device enrollment key: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
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
		Where("device_deployment_id = ?", deploymentID).
		First(&key).Error; err != nil {
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(key.HashedSecret), []byte(secret)); err != nil {
		return nil, fmt.Errorf("invalid device enrollment credential")
	}
	if key.ExpiresAt != nil && key.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("device enrollment credential expired")
	}

	// Best-effort last-used update.
	now := time.Now()
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
	n, err := fmt.Sscanf(credential, "%4s-%d-%d-%s", &prefix, &deploymentID, &keyID, &secret)
	if err != nil || n != 4 {
		return 0, 0, "", fmt.Errorf("invalid device enrollment credential format")
	}
	if prefix != deviceEnrollPrefix {
		return 0, 0, "", fmt.Errorf("invalid device enrollment credential prefix")
	}
	return deploymentID, keyID, secret, nil
}
