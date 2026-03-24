package client

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultAuditLogCleanupInterval = 24 * time.Hour
	defaultAuditLogDeleteBatchSize = 1000
)

type Client struct {
	db                      *db.DB
	encryptionConfig        *encryptionconfig.EncryptionConfiguration
	emailsWithExplicitRoles map[string]types2.Role
	auditLock               sync.Mutex
	auditBuffer             []types.MCPAuditLog
	kickAuditPersist        chan struct{}
	storageClient           kclient.Client
	apiKeyCacheLock         sync.RWMutex
	apiKeyCache             map[[32]byte]apiKeyValidationCacheEntry
	apiKeyCacheTTL          time.Duration
	serviceAccountCacheLock sync.RWMutex
	serviceAccountCache     map[[32]byte]serviceAccountValidationCacheEntry
	serviceAccountCacheTTL  time.Duration
	auditLogCleanupInterval time.Duration
	auditLogDeleteBatchSize int
	oktaGroupMigrationMu    sync.Mutex
	oktaGroupMigrationDone  bool
}

func New(ctx context.Context, db *db.DB, storageClient kclient.Client, encryptionConfig *encryptionconfig.EncryptionConfiguration, ownerEmails, adminEmails []string, auditLogPersistenceInterval time.Duration, auditLogBatchSize, auditLogRetentionDays int) *Client {
	explicitRoleEmailsSet := make(map[string]types2.Role, len(ownerEmails)+len(adminEmails))
	for _, email := range adminEmails {
		explicitRoleEmailsSet[strings.ToLower(email)] = types2.RoleAdmin
	}
	// If a user is explicitly both an admin and owner, they are an owner.
	for _, email := range ownerEmails {
		explicitRoleEmailsSet[strings.ToLower(email)] = types2.RoleOwner
	}
	c := &Client{
		db:                      db,
		encryptionConfig:        encryptionConfig,
		emailsWithExplicitRoles: explicitRoleEmailsSet,
		auditBuffer:             make([]types.MCPAuditLog, 0, 2*auditLogBatchSize),
		kickAuditPersist:        make(chan struct{}),
		storageClient:           storageClient,
		apiKeyCache:             make(map[[32]byte]apiKeyValidationCacheEntry),
		apiKeyCacheTTL:          apiKeyValidationCacheTTL,
		serviceAccountCache:     make(map[[32]byte]serviceAccountValidationCacheEntry),
		serviceAccountCacheTTL:  serviceAccountValidationCacheTTL,
		auditLogCleanupInterval: defaultAuditLogCleanupInterval,
		auditLogDeleteBatchSize: defaultAuditLogDeleteBatchSize,
	}

	go c.runPersistenceLoop(ctx, auditLogPersistenceInterval)
	go c.runPendingStateCleanup(ctx)
	go c.runAPIKeyCacheCleanup(ctx)
	go c.runAuditLogCleanup(ctx, auditLogRetentionDays)
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
	return c.emailsWithExplicitRoles[strings.ToLower(email)]
}

// GetExplicitRoleEmails returns a copy of all emails with explicit roles.
// Used by setup endpoints to list Owner and Admin emails.
func (c *Client) GetExplicitRoleEmails() map[string]types2.Role {
	// No lock needed - map is immutable after construction
	return maps.Clone(c.emailsWithExplicitRoles)
}
