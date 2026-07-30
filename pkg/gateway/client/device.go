package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

const deviceCreationAdvisoryLockID int64 = 0x6f626f7444657669 // "obotDevi"

// DeviceLimit describes the maximum number of devices an installation may have.
// Maximum is ignored when Unlimited is true.
type DeviceLimit struct {
	Maximum   int64
	Unlimited bool
}

// DeviceLimitProvider resolves the current license-derived device limit.
type DeviceLimitProvider interface {
	DeviceLimit(context.Context) (DeviceLimit, error)
}

// DeviceEnrollment is the input to enrolling (or re-enrolling) a device.
// PublicKey is DER SubjectPublicKeyInfo (PKIX) of the device identity key.
type DeviceEnrollment struct {
	DeviceID           string
	MDMConfigurationID uint
	PublicKey          []byte
	Hostname           string
	OS                 string
	OSVersion          string
}

// EnrollDevice registers a device's identity key trust-on-first-use and returns
// the device. Re-enrollment semantics keyed on DeviceID:
//   - same device, same key      -> reactivate and rebind to the configuration
//   - same device, different key  -> rejected (anti-takeover)
//   - new device                  -> created
func (c *Client) EnrollDevice(ctx context.Context, in DeviceEnrollment, deviceLimit DeviceLimit) (*types.Device, error) {
	if !deviceLimit.Unlimited {
		var existing types.Device
		err := c.db.WithContext(ctx).Where("device_id = ?", in.DeviceID).First(&existing).Error
		switch {
		case err == nil:
			return c.enrollDevice(ctx, in, deviceLimit)
		case errors.Is(err, gorm.ErrRecordNotFound):
			// SQLite transactions are deferred, and this flow does not write
			// before counting devices. Serialize local allocations so concurrent
			// count-and-create operations cannot both claim the same seat.
			c.deviceCreationLock.Lock()
			defer c.deviceCreationLock.Unlock()
		default:
			return nil, err
		}
	}

	return c.enrollDevice(ctx, in, deviceLimit)
}

func (c *Client) enrollDevice(ctx context.Context, in DeviceEnrollment, deviceLimit DeviceLimit) (*types.Device, error) {
	var device types.Device
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing types.Device
		err := tx.Where("device_id = ?", in.DeviceID).First(&existing).Error
		switch {
		case err == nil:
			if !bytes.Equal(existing.PublicKey, in.PublicKey) {
				return fmt.Errorf("device %q is already enrolled with a different identity key", in.DeviceID)
			}
			if err := updateEnrolledDevice(tx, &existing, in); err != nil {
				return err
			}
			device = existing
			return nil
		case errors.Is(err, gorm.ErrRecordNotFound):
			if !deviceLimit.Unlimited {
				if err := lockDeviceCreation(tx); err != nil {
					return err
				}

				// Another PostgreSQL replica may have enrolled this DeviceID
				// while this transaction waited for the advisory lock.
				err = tx.Where("device_id = ?", in.DeviceID).First(&existing).Error
				switch {
				case err == nil:
					if !bytes.Equal(existing.PublicKey, in.PublicKey) {
						return fmt.Errorf("device %q is already enrolled with a different identity key", in.DeviceID)
					}
					if err := updateEnrolledDevice(tx, &existing, in); err != nil {
						return err
					}
					device = existing
					return nil
				case !errors.Is(err, gorm.ErrRecordNotFound):
					return err
				}

				deviceCount, err := countDevices(tx)
				if err != nil {
					return fmt.Errorf("failed to count devices: %w", err)
				}
				if deviceCount >= deviceLimit.Maximum {
					return newDeviceLimitError()
				}
			}

			device = types.Device{
				DeviceID:           in.DeviceID,
				MDMConfigurationID: in.MDMConfigurationID,
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

func updateEnrolledDevice(tx *gorm.DB, existing *types.Device, in DeviceEnrollment) error {
	updates := map[string]any{
		"mdm_configuration_id": in.MDMConfigurationID,
		"hostname":             in.Hostname,
		"os":                   in.OS,
		"os_version":           in.OSVersion,
	}
	if err := tx.Model(existing).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to re-enroll device: %w", err)
	}
	existing.MDMConfigurationID = in.MDMConfigurationID
	existing.Hostname = in.Hostname
	existing.OS = in.OS
	existing.OSVersion = in.OSVersion
	return nil
}

func countDevices(tx *gorm.DB) (int64, error) {
	var count int64
	err := tx.Model(new(types.Device)).Count(&count).Error
	return count, err
}

func lockDeviceCreation(tx *gorm.DB) error {
	if tx.Name() == "postgres" {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", deviceCreationAdvisoryLockID).Error; err != nil {
			return fmt.Errorf("failed to lock device creation: %w", err)
		}
	}
	return nil
}

func newDeviceLimitError() error {
	return apitypes.NewErrHTTP(
		http.StatusForbidden,
		"Unable to enroll your device. Please contact your administrator.",
	)
}

// DeviceCount returns the total number of enrolled devices.
func (c *Client) DeviceCount(ctx context.Context) (int64, error) {
	return countDevices(c.db.WithContext(ctx))
}

// GetDeviceByDeviceID looks up an enrolled device by its client-computed ID.
func (c *Client) GetDeviceByDeviceID(ctx context.Context, deviceID string) (*types.Device, error) {
	var device types.Device
	if err := c.db.WithContext(ctx).Where("device_id = ?", deviceID).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

// ListDevices returns the devices enrolled into a configuration, newest first.
func (c *Client) ListDevices(ctx context.Context, configurationID uint) ([]types.Device, error) {
	var devices []types.Device
	if err := c.db.WithContext(ctx).
		Where("mdm_configuration_id = ?", configurationID).
		Order("enrolled_at DESC").
		Find(&devices).Error; err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}
	return devices, nil
}
