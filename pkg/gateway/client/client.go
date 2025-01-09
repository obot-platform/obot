package client

import (
	"strconv"

	"github.com/obot-platform/obot/pkg/gateway/db"
)

type Client struct {
	db          *db.DB
	adminEmails map[string]struct{}
}

func New(db *db.DB, adminEmails []string) *Client {
	adminEmailsSet := make(map[string]struct{}, len(adminEmails))
	for _, email := range adminEmails {
		adminEmailsSet[email] = struct{}{}
	}
	return &Client{
		db:          db,
		adminEmails: adminEmailsSet,
	}
}

func (c *Client) Close() error {
	return c.db.Close()
}

func (c *Client) IsExplicitAdmin(email string) bool {
	_, ok := c.adminEmails[email]
	return ok
}

func firstValue(m map[string][]string, key string) string {
	values := m[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstValueAsInt(m map[string][]string, key string) int {
	value := firstValue(m, key)
	v, _ := strconv.Atoi(value)
	return v
}
