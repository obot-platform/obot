package client

import (
	"context"
	"errors"
	"strconv"

	"gorm.io/gorm"
)

const (
	registryAnonymousAccessEnabledKey = "registry_anonymous_access_enabled"
)

// AnonymousRegistryAccessEnabled reads shared state on every call. Only an absent property
// uses the default of disabled.
func (c *Client) AnonymousRegistryAccessEnabled(ctx context.Context) (bool, error) {
	p, err := c.GetProperty(ctx, registryAnonymousAccessEnabledKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("registry settings unavailable")
	}

	enabled, err := strconv.ParseBool(p.Value)
	if err != nil {
		return false, errors.New("invalid registry settings")
	}
	return enabled, nil
}

// SetAnonymousRegistryAccessEnabled stores the setting through the shared encrypted property upsert.
func (c *Client) SetAnonymousRegistryAccessEnabled(ctx context.Context, enabled bool) error {
	_, err := c.SetProperty(ctx, registryAnonymousAccessEnabledKey, strconv.FormatBool(enabled))
	return err
}
