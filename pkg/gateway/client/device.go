package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

// DeviceEnrollment is the input to enrolling (or re-enrolling) a device.
// PublicKey is DER SubjectPublicKeyInfo (PKIX) of the device identity key.
type DeviceEnrollment struct {
	DeviceID           string
	DeviceDeploymentID uint
	PublicKey          []byte
	Hostname           string
	OS                 string
	OSVersion          string
}

// EnrollDevice registers a device's identity key trust-on-first-use and returns
// the device. Re-enrollment semantics keyed on DeviceID:
//   - same device, same key      -> reactivate and rebind to the deployment
//   - same device, different key  -> rejected (anti-takeover)
//   - new device                  -> created
func (c *Client) EnrollDevice(ctx context.Context, in DeviceEnrollment) (*types.Device, error) {
	var device types.Device
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing types.Device
		err := tx.Where("device_id = ?", in.DeviceID).First(&existing).Error
		switch {
		case err == nil:
			if !bytes.Equal(existing.PublicKey, in.PublicKey) {
				return fmt.Errorf("device %q is already enrolled with a different identity key", in.DeviceID)
			}
			updates := map[string]any{
				"device_deployment_id": in.DeviceDeploymentID,
				"hostname":             in.Hostname,
				"os":                   in.OS,
				"os_version":           in.OSVersion,
			}
			if err := tx.Model(&existing).Updates(updates).Error; err != nil {
				return fmt.Errorf("failed to re-enroll device: %w", err)
			}
			existing.DeviceDeploymentID = in.DeviceDeploymentID
			device = existing
			return nil
		case errors.Is(err, gorm.ErrRecordNotFound):
			device = types.Device{
				DeviceID:           in.DeviceID,
				DeviceDeploymentID: in.DeviceDeploymentID,
				PublicKey:          in.PublicKey,
				Hostname:           in.Hostname,
				OS:                 in.OS,
				OSVersion:          in.OSVersion,
				EnrolledAt:         time.Now(),
			}
			if err := tx.Create(&device).Error; err != nil {
				return fmt.Errorf("failed to enroll device: %w", err)
			}
			return nil
		default:
			return err
		}
	}); err != nil {
		return nil, err
	}
	return &device, nil
}

// GetDeviceByDeviceID looks up an enrolled device by its client-computed ID.
func (c *Client) GetDeviceByDeviceID(ctx context.Context, deviceID string) (*types.Device, error) {
	var device types.Device
	if err := c.db.WithContext(ctx).Where("device_id = ?", deviceID).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

// ListDevices returns the devices enrolled into a deployment, newest first.
func (c *Client) ListDevices(ctx context.Context, deploymentID uint) ([]types.Device, error) {
	var devices []types.Device
	if err := c.db.WithContext(ctx).
		Where("device_deployment_id = ?", deploymentID).
		Order("enrolled_at DESC").
		Find(&devices).Error; err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}
	return devices, nil
}
