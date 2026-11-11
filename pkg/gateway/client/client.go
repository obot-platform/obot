package client

import (
	"context"
	"errors"
	"fmt"
	"math"
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
	defaultAuditLogBatchSize       = 1000
	defaultAuditShutdownTimeout    = 20 * time.Second

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
	auditLogBatchSize int
	auditWriteCtx     context.Context
	auditWriteCancel  context.CancelFunc
	auditLoopsCancel  context.CancelFunc
	auditLoops        sync.WaitGroup
	auditWriteLock    sync.RWMutex
	auditClosed       bool

	egressAuditLock        sync.Mutex
	egressAuditPersistLock sync.Mutex
	egressAuditBuffer      []types.EgressAuditLog
	egressAuditBatchSize   int
	kickEgressAuditPersist chan struct{}

	db                        *db.DB
	encryptionConfig          *encryptionconfig.EncryptionConfiguration
	auditLock                 sync.Mutex
	auditPersistLock          sync.Mutex
	auditBuffer               []types.MCPAuditLog
	mcpAuditDisabled          bool
	kickAuditPersist          chan struct{}
	enforcementLock           sync.Mutex
	enforcementPersistLock    sync.Mutex
	enforcementBuffer         []types.EnforcementDecisionLog
	kickEnforcementPersist    chan struct{}
	llmAuditEntries           chan llmAuditEntry
	llmAuditBatchSize         int
	llmAuditEnabled           bool
	llmAuditPartial           []llmAuditEntry
	storageClient             kclient.Client
	apiKeyCacheLock           sync.RWMutex
	apiKeyCache               map[[32]byte]apiKeyValidationCacheEntry
	apiKeyCacheTTL            time.Duration
	serviceAccountCacheLock   sync.RWMutex
	serviceAccountCache       map[[32]byte]serviceAccountValidationCacheEntry
	serviceAccountCacheTTL    time.Duration
	deviceCreationLock        sync.Mutex
	auditLogCleanupInterval   time.Duration
	auditLogRetentionDays     int
	llmAuditLogRetentionDays  int
	auditLogDeleteBatchSize   int
	deviceScanCleanupInterval time.Duration
	deviceScanRetentionDays   int
	deviceScanDeleteBatchSize int
	mcpOAuthTokenTrigger      func(context.Context, string) error
	groupRefresh              singleflight.Group
	groupCooldown             groupRefreshCooldown
	kickLifecycleDelivery     chan struct{}
	// lockGates admit one caller in this process at a time to each lock taken with withGate.
	lockGates sync.Map
	// startOnce keeps StartCleanups from starting the cleanups more than once.
	startOnce sync.Once
}

func New(ctx context.Context, db *db.DB, storageClient kclient.Client, encryptionConfig *encryptionconfig.EncryptionConfiguration, mcpOAuthTokenTrigger func(context.Context, string) error, auditLogPersistenceInterval time.Duration, auditLogBatchSize, auditLogRetentionDays, llmAuditLogRetentionDays, deviceScanRetentionDays int, llmAuditEnabled bool) *Client {
	auditLogBatchSize, initialCapacity := auditBatchCapacity(auditLogBatchSize)
	c := &Client{
		auditLogBatchSize:      auditLogBatchSize,
		egressAuditBatchSize:   auditLogBatchSize,
		kickEgressAuditPersist: make(chan struct{}, 1),
		db:                     db,
		encryptionConfig:       encryptionConfig,
		// Avoid eagerly allocating large configured batches; buffers grow on demand.
		auditBuffer:               make([]types.MCPAuditLog, 0, min(initialCapacity, 2*defaultAuditLogBatchSize)),
		kickAuditPersist:          make(chan struct{}, 1),
		enforcementBuffer:         make([]types.EnforcementDecisionLog, 0, min(initialCapacity, 2*defaultAuditLogBatchSize)),
		kickEnforcementPersist:    make(chan struct{}, 1),
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
		auditLogRetentionDays:     auditLogRetentionDays,
		llmAuditLogRetentionDays:  llmAuditLogRetentionDays,
		auditLogDeleteBatchSize:   defaultAuditLogDeleteBatchSize,
		deviceScanCleanupInterval: defaultDeviceScanCleanupInterval,
		deviceScanRetentionDays:   deviceScanRetentionDays,
		deviceScanDeleteBatchSize: defaultDeviceScanDeleteBatchSize,
		kickLifecycleDelivery:     make(chan struct{}, 1),
	}

	c.startAuditLoops(ctx, auditLogPersistenceInterval)
	go c.runAPIKeyCacheCleanup(ctx)
	go c.runUserLifecycleEventDelivery(ctx)
	return c
}

// StartCleanups starts the loops that delete expired and retained data from the database. Only one replica needs to
// run them, so the controller starts them once this replica is elected leader. Calling it again does nothing.
func (c *Client) StartCleanups(ctx context.Context, entitlements EntitlementProvider) {
	c.startOnce.Do(func() {
		go c.runPendingStateCleanup(ctx)
		go c.runTokenCleanup(ctx)
		go c.runRetentionCleanup(ctx, entitlements)
		go c.runDeviceScanCleanup(ctx, c.deviceScanRetentionDays)
		go c.runSCIMGroupDeletionMarkExpiry(ctx)
	})
}

func (c *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), defaultAuditShutdownTimeout)
	defer cancel()

	return c.CloseContext(ctx)
}

func auditBatchCapacity(batchSize int) (int, int) {
	if batchSize <= 0 {
		batchSize = defaultAuditLogBatchSize
	}
	// Saturate before doubling; an extreme configuration must not wrap or allocate eagerly.
	batchSize = min(batchSize, math.MaxInt/2)

	return batchSize, 2 * batchSize
}

func (c *Client) startAuditLoops(ctx context.Context, interval time.Duration) {
	c.auditWriteCtx, c.auditWriteCancel = context.WithCancel(context.Background())
	ctx, c.auditLoopsCancel = context.WithCancel(ctx)
	for _, loop := range []func(context.Context, time.Duration){
		c.runEgressAuditPersistenceLoop,
		c.runMCPAuditLogPersistenceLoop,
		c.runEnforcementDecisionPersistenceLoop,
		func(ctx context.Context, interval time.Duration) {
			c.runLLMAuditPersistenceLoop(ctx, c.llmAuditBatchSize, interval)
		},
	} {
		c.auditLoops.Go(func() {
			loop(ctx, interval)
		})
	}
}

// writeAuditLogs tracks even explicit flushes so shutdown can cancel and join every writer.
func (c *Client) writeAuditLogs(ctx context.Context, write func(context.Context) error) error {
	c.auditWriteLock.RLock()
	defer c.auditWriteLock.RUnlock()
	if c.auditClosed {
		return errors.New("audit writer is closed")
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// Stop loops without interrupting active commits; the shutdown budget cancels writers.
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	stop := context.AfterFunc(c.auditWriteCtx, cancel)
	defer stop()
	defer cancel()
	if c.auditWriteCtx.Err() != nil {
		cancel()
	}

	return write(ctx)
}

// CloseContext stops audit loops, joins their writes, and drains all queues within one budget.
// Drivers must honor context cancellation; DB closure follows the last cooperative writer.
func (c *Client) CloseContext(ctx context.Context) error {
	if c.auditLoopsCancel != nil {
		c.auditLoopsCancel()
	}

	stop := context.AfterFunc(ctx, c.auditWriteCancel)
	defer stop()
	if ctx.Err() != nil {
		c.auditWriteCancel()
	}
	c.auditLoops.Wait()

	// Exclusive ownership also joins explicit flushes and prevents writes after DB closure.
	c.auditWriteLock.Lock()
	defer c.auditWriteLock.Unlock()
	if c.auditClosed {
		return nil
	}
	c.auditClosed = true
	defer c.auditWriteCancel()

	var errs []error
	if err := c.flushEgressAuditLogs(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to persist egress audit logs: %w", err))
	}
	if err := c.persistMCPAuditLogsContext(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to persist audit logs: %w", err))
	}
	if err := c.persistEnforcementDecisionsContext(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to persist enforcement decisions: %w", err))
	}
	if err := c.persistQueuedLLMAuditLogs(ctx); err != nil {
		errs = append(errs, fmt.Errorf("failed to persist LLM audit logs: %w", err))
	}

	return errors.Join(append(errs, ctx.Err(), c.db.Close())...)
}
