package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Client struct {
	db                     *db.DB
	encryptionConfig       *encryptionconfig.EncryptionConfiguration
	emailsWithExplictRoles map[string]types2.Role
	auditLock              sync.Mutex
	auditBuffer            []types.MCPAuditLog
	kickAuditPersist       chan struct{}
	storageClient          kclient.Client
}

func New(ctx context.Context, db *db.DB, storageClient kclient.Client, encryptionConfig *encryptionconfig.EncryptionConfiguration, ownerEmails, adminEmails []string, auditLogPersistenceInterval time.Duration, auditLogBatchSize int) *Client {
	adminEmailsSet := make(map[string]types2.Role, len(ownerEmails)+len(adminEmails))
	for _, email := range ownerEmails {
		adminEmailsSet[email] = types2.RoleOwner
	}
	for _, email := range adminEmails {
		adminEmailsSet[email] = types2.RoleAdmin
	}
	c := &Client{
		db:                     db,
		encryptionConfig:       encryptionConfig,
		emailsWithExplictRoles: adminEmailsSet,
		auditBuffer:            make([]types.MCPAuditLog, 0, 2*auditLogBatchSize),
		kickAuditPersist:       make(chan struct{}),
		storageClient:          storageClient,
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

func (c *Client) HasExplicitRole(email string) types2.Role {
	return c.emailsWithExplictRoles[email]
}
