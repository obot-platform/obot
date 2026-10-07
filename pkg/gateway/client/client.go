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
	"golang.org/x/sync/singleflight"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultAuditLogCleanupInterval = 24 * time.Hour
	defaultAuditLogDeleteBatchSize = 1000

	defaultDeviceScanCleanupInterval = 24 * time.Hour
	defaultDeviceScanDeleteBatchSize = 100

	// DefaultUserLimit is the maximum number of users allowed when no
	// license-derived user-limit provider is configured.
	DefaultUserLimit = 100

	// DefaultDeviceLimit is the maximum number of devices allowed when no
	// license-derived device-limit provider is configured.
	DefaultDeviceLimit = 100
)

// SystemLimit describes the maximum number of a resource an installation may have.
// Maximum is ignored when Unlimited is true.
type SystemLimit struct {
	Maximum   int64
	Unlimited bool
}

type Client struct {
	db                        *db.DB
	encryptionConfig          *encryptionconfig.EncryptionConfiguration
	emailsWithExplicitRoles   map[string]types2.Role
	auditLock                 sync.Mutex
	auditBuffer               []types.MCPAuditLog
	kickAuditPersist          chan struct{}
	enforcementLock           sync.Mutex
	enforcementBuffer         []types.EnforcementDecisionLog
	kickEnforcementPersist    chan struct{}
	llmAuditEntries           chan llmAuditEntry
	llmAuditBatchSize         int
	llmAuditEnabled           bool
	storageClient             kclient.Client
	apiKeyCacheLock           sync.RWMutex
	apiKeyCache               map[[32]byte]apiKeyValidationCacheEntry
	apiKeyCacheTTL            time.Duration
	serviceAccountCacheLock   sync.RWMutex
	serviceAccountCache       map[[32]byte]serviceAccountValidationCacheEntry
	serviceAccountCacheTTL    time.Duration
	deviceCreationLock        sync.Mutex
	auditLogCleanupInterval   time.Duration
	auditLogDeleteBatchSize   int
	deviceScanCleanupInterval time.Duration
	deviceScanDeleteBatchSize int
	mcpAuditLogRetentionDays  int
	llmAuditLogRetentionDays  int
	startAuditCleanupOnce     sync.Once
	mcpOAuthTokenTrigger      func(context.Context, string) error
	groupRefresh              singleflight.Group
	groupCooldown             groupRefreshCooldown
	kickLifecycleDelivery     chan struct{}
}

func New(ctx context.Context, db *db.DB, storageClient kclient.Client, encryptionConfig *encryptionconfig.EncryptionConfiguration, mcpOAuthTokenTrigger func(context.Context, string) error, ownerEmails, adminEmails []string, auditLogPersistenceInterval time.Duration, auditLogBatchSize, auditLogRetentionDays, llmAuditLogRetentionDays, deviceScanRetentionDays int, llmAuditEnabled bool) *Client {
	explicitRoleEmailsSet := make(map[string]types2.Role, len(ownerEmails)+len(adminEmails))
	for _, email := range adminEmails {
		explicitRoleEmailsSet[strings.ToLower(email)] = types2.RoleAdmin
	}
	// If a user is explicitly both an admin and owner, they are an owner.
	for _, email := range ownerEmails {
		explicitRoleEmailsSet[strings.ToLower(email)] = types2.RoleOwner
	}
	c := &Client{
		db:                        db,
		encryptionConfig:          encryptionConfig,
		emailsWithExplicitRoles:   explicitRoleEmailsSet,
		auditBuffer:               make([]types.MCPAuditLog, 0, 2*auditLogBatchSize),
		kickAuditPersist:          make(chan struct{}),
		enforcementBuffer:         make([]types.EnforcementDecisionLog, 0, 2*auditLogBatchSize),
		kickEnforcementPersist:    make(chan struct{}),
		storageClient:             storageClient,
		mcpOAuthTokenTrigger:      mcpOAuthTokenTrigger,
		apiKeyCache:               make(map[[32]byte]apiKeyValidationCacheEntry),
		apiKeyCacheTTL:            apiKeyValidationCacheTTL,
		serviceAccountCache:       make(map[[32]byte]serviceAccountValidationCacheEntry),
		serviceAccountCacheTTL:    serviceAccountValidationCacheTTL,
		llmAuditEntries:           make(chan llmAuditEntry, defaultLLMAuditLogBufferSize),
		llmAuditBatchSize:         defaultLLMAuditLogBatchSize,
		llmAuditEnabled:           llmAuditEnabled,
		auditLogCleanupInterval:   defaultAuditLogCleanupInterval,
		auditLogDeleteBatchSize:   defaultAuditLogDeleteBatchSize,
		deviceScanCleanupInterval: defaultDeviceScanCleanupInterval,
		deviceScanDeleteBatchSize: defaultDeviceScanDeleteBatchSize,
		kickLifecycleDelivery:     make(chan struct{}, 1),
		mcpAuditLogRetentionDays:  auditLogRetentionDays,
		llmAuditLogRetentionDays:  llmAuditLogRetentionDays,
	}

	go c.runMCPAuditLogPersistenceLoop(ctx, auditLogPersistenceInterval)
	go c.runEnforcementDecisionPersistenceLoop(ctx, auditLogPersistenceInterval)
	go c.runLLMAuditPersistenceLoop(ctx, c.llmAuditBatchSize, auditLogPersistenceInterval)
	go c.runPendingStateCleanup(ctx)
	go c.runTokenCleanup(ctx)
	go c.runAPIKeyCacheCleanup(ctx)
	go c.runDeviceScanCleanup(ctx, deviceScanRetentionDays)
	go c.runUserLifecycleEventDelivery(ctx)
	go c.runSCIMGroupDeletionMarkExpiry(ctx)
	return c
}

// StartAuditCleanup starts deleting audit logs and revoked API keys once they're past retention.
// Only one replica needs to delete them, so the controller starts it once this replica is elected leader.
// The licensed retention limit is looked up again on every pass. Calls after the first do nothing.
func (c *Client) StartAuditCleanup(ctx context.Context, limits RetentionLimits) {
	c.startAuditCleanupOnce.Do(func() {
		go c.runAuditCleanup(ctx, limits)
	})
}

func (c *Client) Close() error {
	var errs []error
	if err := c.persistMCPAuditLogs(); err != nil {
		errs = append(errs, fmt.Errorf("failed to persist audit logs: %w", err))
	}
	if err := c.persistEnforcementDecisions(); err != nil {
		errs = append(errs, fmt.Errorf("failed to persist enforcement decisions: %w", err))
	}
	if err := c.persistQueuedLLMAuditLogs(); err != nil {
		errs = append(errs, fmt.Errorf("failed to persist LLM audit logs: %w", err))
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
