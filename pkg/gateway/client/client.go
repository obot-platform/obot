package client

import (
	"strconv"

	"github.com/otto8-ai/otto8/pkg/gateway/db"
	"k8s.io/apiserver/pkg/authentication/authenticator"
)

type Client struct {
	db          *db.DB
	adminEmails map[string]struct{}
	nextAuth    authenticator.Request
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
