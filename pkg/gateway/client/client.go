package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
)

type Client struct {
	db               *db.DB
	encryptionConfig *encryptionconfig.EncryptionConfiguration
	adminEmails      map[string]struct{}
	auditLock        sync.Mutex
	auditBuffer      []types.MCPAuditLog
	kickAuditPersist chan struct{}
}

func New(ctx context.Context, db *db.DB, encryptionConfig *encryptionconfig.EncryptionConfiguration, adminEmails []string, auditLogPersistenceInterval time.Duration, auditLogBatchSize int) *Client {
	adminEmailsSet := make(map[string]struct{}, len(adminEmails))
	for _, email := range adminEmails {
		adminEmailsSet[email] = struct{}{}
	}
	c := &Client{
		db:               db,
		encryptionConfig: encryptionConfig,
		adminEmails:      adminEmailsSet,
		auditBuffer:      make([]types.MCPAuditLog, 0, 2*auditLogBatchSize),
		kickAuditPersist: make(chan struct{}),
	}

	go c.runPersistenceLoop(ctx, auditLogPersistenceInterval)
	return c
}

func (c *Client) Close() error {
	var errs []error
	if err := c.persistAuditLogs(); err != nil {
		errs = append(errs, fmt.Errorf("failed to persist audit logs: %w", err))
	}

	return errors.Join(append(errs, c.db.Close())...)
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
