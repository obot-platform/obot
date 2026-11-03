package client

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

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

type Client struct {
	egressAuditLock        sync.Mutex
	egressAuditPersistLock sync.Mutex
	egressAuditBuffer      []types.EgressAuditLog
	egressAuditBatchSize   int
	kickEgressAuditPersist chan struct{}
	egressAuditCancel      context.CancelFunc
	egressAuditDone        chan struct{}

	db                        *db.DB
	encryptionConfig          *encryptionconfig.EncryptionConfiguration
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
	mcpOAuthTokenTrigger      func(context.Context, string) error
	groupRefresh              singleflight.Group
	groupCooldown             groupRefreshCooldown
	kickLifecycleDelivery     chan struct{}
	// claudeProvisionGateSlot admits one caller at a time to WithClaudeProvisionLock.
	claudeProvisionGateOnce sync.Once
	claudeProvisionGateSlot chan struct{}
}

func New(ctx context.Context, db *db.DB, storageClient kclient.Client, encryptionConfig *encryptionconfig.EncryptionConfiguration, mcpOAuthTokenTrigger func(context.Context, string) error, auditLogPersistenceInterval time.Duration, auditLogBatchSize, auditLogRetentionDays, llmAuditLogRetentionDays, deviceScanRetentionDays int, llmAuditEnabled bool) *Client {
	egressCtx, egressCancel := context.WithCancel(ctx)
	c := &Client{
		egressAuditBatchSize:      auditLogBatchSize,
		kickEgressAuditPersist:    make(chan struct{}, 1),
		egressAuditCancel:         egressCancel,
		egressAuditDone:           make(chan struct{}),
		db:                        db,
		encryptionConfig:          encryptionConfig,
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
	}

	go c.runEgressAuditPersistenceLoop(egressCtx, auditLogPersistenceInterval)
	go c.runMCPAuditLogPersistenceLoop(ctx, auditLogPersistenceInterval)
	go c.runEnforcementDecisionPersistenceLoop(ctx, auditLogPersistenceInterval)
	go c.runLLMAuditPersistenceLoop(ctx, c.llmAuditBatchSize, auditLogPersistenceInterval)
	go c.runPendingStateCleanup(ctx)
	go c.runTokenCleanup(ctx)
	go c.runAPIKeyCacheCleanup(ctx)
	go c.runRetentionCleanup(ctx, auditLogRetentionDays, llmAuditLogRetentionDays)
	go c.runDeviceScanCleanup(ctx, deviceScanRetentionDays)
	go c.runUserLifecycleEventDelivery(ctx)
	go c.runSCIMGroupDeletionMarkExpiry(ctx)
	return c
}

func (c *Client) Close() error {
	var errs []error
	if c.egressAuditCancel != nil {
		c.egressAuditCancel()
		<-c.egressAuditDone
	}
	if err := c.FlushEgressAuditLogs(); err != nil {
		errs = append(errs, fmt.Errorf("failed to persist egress audit logs: %w", err))
	}
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
