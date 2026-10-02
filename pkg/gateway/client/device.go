package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

const (
	deviceCreationAdvisoryLockID int64 = 0x6f626f7444657669 // "obotDevi"
)

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

// DeviceHostnamesByDeviceID resolves registry hostnames for a set of device IDs in one query.
// It returns a map keyed by device ID; IDs with no enrolled device, or whose device has never
// reported a hostname, are absent. Callers use it to label audit-log rows, which carry a
// server-stamped device ID but no searchable hostname of their own.
func (c *Client) DeviceHostnamesByDeviceID(ctx context.Context, deviceIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(deviceIDs))
	if len(deviceIDs) == 0 {
		return out, nil
	}
	var rows []struct {
		DeviceID string
		Hostname string
	}
	if err := c.db.WithContext(ctx).
		Model(&types.Device{}).
		Select("device_id, hostname").
		Where("device_id IN ? AND hostname != ''", deviceIDs).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to resolve device hostnames: %w", err)
	}
	for _, r := range rows {
		out[r.DeviceID] = r.Hostname
	}
	return out, nil
}

// DeviceIDsMatchingHostnames returns the device IDs whose registry hostname matches any of the
// given patterns (case-insensitive substring). An empty or nil result set means "no constraint",
// so callers must not pass an empty slice when they mean to filter nothing.
func (c *Client) DeviceIDsMatchingHostnames(ctx context.Context, patterns []string) ([]string, error) {
	seen := make(map[string]struct{}, len(patterns))
	var out []string
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		db := c.db.WithContext(ctx).Model(&types.Device{})
		// ILIKE is Postgres-only, and SQLite (used by tests) rejects it outright. SQLite's LIKE
		// is already case-insensitive for ASCII and Postgres takes the ILIKE branch, so both
		// dialects match case-insensitively. applyAuditLogSearch picks its operator the same way.
		like := "LIKE"
		if db.Name() == "postgres" {
			like = "ILIKE"
		}
		var ids []string
		if err := db.
			Where(`hostname `+like+` ? ESCAPE '\'`, "%"+escapeLikePattern(p)+"%").
			Pluck("device_id", &ids).Error; err != nil {
			return nil, fmt.Errorf("failed to match device hostnames: %w", err)
		}
		for _, id := range ids {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out, nil
}

// escapeLikePattern neutralises the LIKE/ILIKE wildcards in a user-supplied search term so a
// query containing % or _ matches literally instead of turning into a wildcard. It assumes the
// query pairs it with an explicit `ESCAPE '\'`, which both Postgres and SQLite require before
// they will honour a backslash.
func escapeLikePattern(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

// RefreshDeviceFromScan updates the registry hostname and last-seen time for a device that just
// submitted a scan. Scans are periodic, so this keeps a renamed machine's current hostname in the
// registry; without it Device.Hostname stays at whatever enrollment recorded.
//
// A device that is not enrolled is left alone rather than created: enrollment owns the MDM
// configuration assignment, and inventing a row here would produce a device that cannot
// authenticate. A scan with no hostname only refreshes last-seen.
func (c *Client) RefreshDeviceFromScan(ctx context.Context, deviceID, hostname string, seenAt time.Time) error {
	if deviceID == "" {
		return nil
	}
	updates := map[string]any{"last_seen_at": seenAt}
	if hostname != "" {
		updates["hostname"] = hostname
	}
	return c.db.WithContext(ctx).
		Model(&types.Device{}).
		Where("device_id = ?", deviceID).
		Updates(updates).Error
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
