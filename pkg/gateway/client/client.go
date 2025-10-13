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

	// Temporary user cache for bootstrap setup flow
	tempUserCacheLock sync.RWMutex
	tempUserCache     *TempUserCacheEntry
}

// TempUserCacheEntry represents a temporarily cached user during the bootstrap setup flow.
type TempUserCacheEntry struct {
	UserID           uint
	Username         string
	Email            string
	Role             types2.Role
	IconURL          string
	AuthProviderName string
	AuthProviderNS   string
	CachedAt         time.Time
}

func New(ctx context.Context, db *db.DB, storageClient kclient.Client, encryptionConfig *encryptionconfig.EncryptionConfiguration, ownerEmails, adminEmails []string, auditLogPersistenceInterval time.Duration, auditLogBatchSize int) *Client {
	explicitRoleEmailsSet := make(map[string]types2.Role, len(ownerEmails)+len(adminEmails))
	for _, email := range adminEmails {
		explicitRoleEmailsSet[email] = types2.RoleAdmin
	}
	// If a user is explicitly both an admin and owner, they are an owner.
	for _, email := range ownerEmails {
		explicitRoleEmailsSet[email] = types2.RoleOwner
	}
	c := &Client{
		db:                     db,
		encryptionConfig:       encryptionConfig,
		emailsWithExplictRoles: explicitRoleEmailsSet,
		auditBuffer:            make([]types.MCPAuditLog, 0, 2*auditLogBatchSize),
		kickAuditPersist:       make(chan struct{}),
		storageClient:          storageClient,
		tempUserCache:          nil, // No cached user initially
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

// SetTempUserCache stores a temporary user in the cache for the bootstrap setup flow.
// Returns an error if a user is already cached.
func (c *Client) SetTempUserCache(user *types.User, authProviderName, authProviderNS string) error {
	c.tempUserCacheLock.Lock()
	defer c.tempUserCacheLock.Unlock()

	if c.tempUserCache != nil {
		return fmt.Errorf("temporary user already cached: %s", c.tempUserCache.Email)
	}

	c.tempUserCache = &TempUserCacheEntry{
		UserID:           user.ID,
		Username:         user.Username,
		Email:            user.Email,
		Role:             user.Role,
		IconURL:          user.IconURL,
		AuthProviderName: authProviderName,
		AuthProviderNS:   authProviderNS,
		CachedAt:         time.Now(),
	}

	return nil
}

// GetTempUserCache retrieves the cached temporary user, if one exists.
// Returns nil if no user is cached.
func (c *Client) GetTempUserCache() *TempUserCacheEntry {
	c.tempUserCacheLock.RLock()
	defer c.tempUserCacheLock.RUnlock()

	if c.tempUserCache == nil {
		return nil
	}

	// Return a copy to prevent external modification
	cached := *c.tempUserCache
	return &cached
}

// ClearTempUserCache removes the cached temporary user.
func (c *Client) ClearTempUserCache() {
	c.tempUserCacheLock.Lock()
	defer c.tempUserCacheLock.Unlock()

	c.tempUserCache = nil
}

// GetExplicitRoleEmails returns a copy of all emails with explicit roles.
// Used by setup endpoints to list Owner and Admin emails.
func (c *Client) GetExplicitRoleEmails() map[string]types2.Role {
	// No lock needed - map is immutable after construction
	result := make(map[string]types2.Role, len(c.emailsWithExplictRoles))
	for email, role := range c.emailsWithExplictRoles {
		result[email] = role
	}
	return result
}
