package client

import (
	"context"
)

func (c *Client) DeleteSessionsForUser(ctx context.Context, emailHash, userHash, tablePrefix string) error {
	return c.db.WithContext(ctx).Exec(
		"DELETE FROM "+tablePrefix+"sessions WHERE \"user\" = decode(?, 'hex') AND \"email\" = decode(?, 'hex')",
		userHash,
		emailHash,
	).Error
}

func (c *Client) DeleteSessionsForUserExceptCurrent(ctx context.Context, emailHash, userHash, tablePrefix, currentSessionID string) error {
	return c.db.WithContext(ctx).Exec(
		"DELETE FROM "+tablePrefix+"sessions WHERE key NOT LIKE ? AND \"user\" = decode(?, 'hex') AND \"email\" = decode(?, 'hex')",
		currentSessionID+"%",
		userHash,
		emailHash,
	).Error
}
