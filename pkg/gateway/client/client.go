package client

import (
	"github.com/obot-platform/obot/pkg/gateway/db"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

type Client struct {
	db          *db.DB
	transformer value.Transformer
	adminEmails map[string]struct{}
}

func New(db *db.DB, encryptionConfig *encryptionconfig.EncryptionConfiguration, adminEmails []string) *Client {
	adminEmailsSet := make(map[string]struct{}, len(adminEmails))
	for _, email := range adminEmails {
		adminEmailsSet[email] = struct{}{}
	}
	var transformer value.Transformer
	if encryptionConfig != nil {
		transformer = encryptionConfig.Transformers[gr]
	}
	return &Client{
		db:          db,
		transformer: transformer,
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
