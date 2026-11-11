package executionbudget

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

var (
	OutcomeWaitingCancelled        = Outcome{Code: "waiting_cancelled"}
	OutcomePreUpstreamCancelled    = Outcome{Code: "pre_upstream_cancelled", Retryable: true}
	OutcomePreUpstreamRejected     = Outcome{Code: "pre_upstream_rejected"}
	OutcomeSnapshotRequired        = Outcome{Code: "snapshot_required", RetainLease: true}
	OutcomeSnapshotRecoveryFailed  = Outcome{Code: "snapshot_recovery_failed"}
	OutcomeCompileFailed           = Outcome{Code: "compile_failed"}
	OutcomeCancelledBeforeUpstream = Outcome{Code: "cancelled_before_upstream", Retryable: true}
	OutcomeBusinessResult          = Outcome{Code: "business_result", UpstreamAttempted: true, BusinessResult: true}
	OutcomeUncertainDeadline       = Outcome{Code: "uncertain_deadline", UpstreamAttempted: true}
	OutcomeUncertainCancel         = Outcome{Code: "uncertain_cancel", UpstreamAttempted: true}
	OutcomeTransportUnknown        = Outcome{Code: "transport_unknown", UpstreamAttempted: true}
	OutcomeWorkerLost              = Outcome{Code: "worker_lost", UpstreamAttempted: true}
	OutcomeResponseLimitUncertain  = Outcome{Code: "response_limit_uncertain", UpstreamAttempted: true}
	OutcomeObotLost                = Outcome{Code: "obot_lost", UpstreamAttempted: true}
)

type BudgetContext struct {
	LeaseID             string `json:"leaseID"`
	LeaseIssuedAtUnixMs int64  `json:"leaseIssuedAtUnixMs"`
	LeaseDeadlineUnixMs int64  `json:"leaseDeadlineUnixMs"`
	FenceDeadlineUnixMs int64  `json:"fenceDeadlineUnixMs"`
	DispatchedAtUnixMs  int64  `json:"dispatchedAtUnixMs"`
	BudgetKeyHash       string `json:"budgetKeyHash"`
	ClockSkewLimitMs    int64  `json:"clockSkewLimitMs"`
	FencingMarginMs     int64  `json:"fencingMarginMs"`
}

type AcquireRequest struct {
	Identity       BudgetIdentity
	OperationKey   string
	Deadline       time.Time
	ObotInstanceID string
}

type Acquisition struct {
	Context      BudgetContext
	ReleaseToken string
}

type Outcome struct {
	Code              string
	UpstreamAttempted bool
	BusinessResult    bool
	RetainLease       bool
	ResponseBytes     int64
	Retryable         bool
}

type Manager struct {
	db      *gorm.DB
	config  Config
	stop    chan struct{}
	admitMu sync.Mutex
}

func NewManager(db *gorm.DB, config Config) (*Manager, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	if db.Name() != "postgres" {
		return nil, fmt.Errorf("%w: PostgreSQL is required", ErrStorageUnavailable)
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Manager{db: db, config: config, stop: make(chan struct{})}, nil
}

func (manager *Manager) Start(ctx context.Context) (func(), error) {
	if manager == nil {
		return nil, ErrStorageUnavailable
	}
	child, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(manager.config.AdmissionInterval)
		defer ticker.Stop()
		for {
			select {
			case <-child.Done():
				return
			case <-manager.stop:
				return
			case <-ticker.C:
				_, _ = manager.AdmitOne(child)
			}
		}
	}()
	return func() {
		close(manager.stop)
		cancel()
		<-done
	}, nil
}

func (manager *Manager) Acquire(ctx context.Context, request AcquireRequest) (Acquisition, error) {
	if request.Identity.BudgetKeyHash == "" || request.Identity.BudgetSourceHash == "" || request.Identity.OperationKeyHash == "" {
		return Acquisition{}, ErrBudgetIdentityUnavailable
	}
	now := time.Now()
	if request.Deadline.Before(now.Add(manager.config.FencingMargin + time.Millisecond)) {
		return Acquisition{}, ErrWaitDeadlineExceeded
	}
	requestID, token, err := randomSecretPair()
	if err != nil {
		return Acquisition{}, err
	}
	waitDeadline := now.Add(manager.config.MaxWait)
	if waitDeadline.After(request.Deadline) {
		waitDeadline = request.Deadline
	}
	if _, err = manager.enqueue(ctx, request, requestID, token, waitDeadline); err != nil {
		return Acquisition{}, err
	}

	ticker := time.NewTicker(minDuration(manager.config.AdmissionInterval, 20*time.Millisecond))
	defer ticker.Stop()
	for {
		if _, err = manager.AdmitOne(ctx); err != nil {
			return Acquisition{}, err
		}
		acquisition, state, err := manager.waitResult(ctx, requestID, token)
		if err == nil && state.State == types.OpenAPIRequestStateActive {
			return acquisition, nil
		}
		if err != nil && !errors.Is(err, ErrLeaseNotFound) && !errors.Is(err, ErrInvalidReleaseToken) {
			return Acquisition{}, err
		}
		select {
		case <-ctx.Done():
			_ = manager.CancelWaiting(context.WithoutCancel(ctx), requestID, token)
			return Acquisition{}, ctx.Err()
		case <-time.After(time.Until(waitDeadline)):
			_ = manager.CancelWaiting(context.WithoutCancel(ctx), requestID, token)
			return Acquisition{}, ErrWaitDeadlineExceeded
		case <-ticker.C:
		}
	}
}

func (manager *Manager) waitResult(ctx context.Context, requestID, token string) (Acquisition, types.OpenAPIExecutionRequest, error) {
	var request types.OpenAPIExecutionRequest
	err := manager.db.WithContext(ctx).Where("request_id = ?", requestID).Take(&request).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Acquisition{}, request, ErrLeaseNotFound
	}
	if err != nil {
		return Acquisition{}, request, err
	}
	if request.State != types.OpenAPIRequestStateActive {
		return Acquisition{}, request, ErrLeaseNotFound
	}
	if request.ReleaseTokenHash != hashReleaseToken(token) {
		return Acquisition{}, request, ErrInvalidReleaseToken
	}
	return Acquisition{
		Context: BudgetContext{
			LeaseID:             request.RequestID,
			LeaseIssuedAtUnixMs: request.AdmittedAt.UTC().UnixMilli(),
			LeaseDeadlineUnixMs: request.Deadline.UTC().UnixMilli(),
			FenceDeadlineUnixMs: request.Deadline.Add(-manager.config.FencingMargin).UTC().UnixMilli(),
			DispatchedAtUnixMs:  time.Now().UTC().UnixMilli(),
			BudgetKeyHash:       request.BudgetKeyHash,
			ClockSkewLimitMs:    (manager.config.FencingMargin / 2).Milliseconds(),
			FencingMarginMs:     manager.config.FencingMargin.Milliseconds(),
		},
		ReleaseToken: token,
	}, request, nil
}

func (manager *Manager) Release(ctx context.Context, leaseID, releaseToken string, outcome Outcome) error {
	return manager.transaction(ctx, func(tx *gorm.DB) error {
		request, err := lockedRequest(tx, leaseID)
		if err != nil {
			return err
		}
		if request.ReleaseTokenHash != hashReleaseToken(releaseToken) {
			return ErrInvalidReleaseToken
		}
		if request.State != types.OpenAPIRequestStateActive {
			return nil
		}
		if outcome.RetainLease {
			return nil
		}
		if !outcomeAllowsImmediateRelease(outcome) {
			return ErrUncertainOutcome
		}
		finishedAt := time.Now().UTC()
		var state string
		switch {
		case outcome.BusinessResult:
			state = types.OpenAPIRequestStateCompleted
		case outcome.Retryable:
			state = types.OpenAPIRequestStateCancelled
		default:
			state = types.OpenAPIRequestStateFailed
		}
		result := tx.Model(&types.OpenAPIExecutionRequest{}).
			Where("id = ? AND state = ?", request.ID, types.OpenAPIRequestStateActive).
			Updates(map[string]any{
				"state": state, "finished_at": finishedAt, "outcome_code": outcome.Code,
				"response_bytes": outcome.ResponseBytes, "retryable": outcome.Retryable,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return reconcileAndCleanupState(tx, request, manager.config.MaxTerminalRequests)
	})
}

func (manager *Manager) CancelWaiting(ctx context.Context, requestID, releaseToken string) error {
	return manager.transaction(ctx, func(tx *gorm.DB) error {
		request, err := lockedRequest(tx, requestID)
		if err != nil {
			return err
		}
		if request.ReleaseTokenHash != hashReleaseToken(releaseToken) {
			return ErrInvalidReleaseToken
		}
		if request.State != types.OpenAPIRequestStateWaiting {
			return nil
		}
		result := tx.Model(&types.OpenAPIExecutionRequest{}).
			Where("id = ? AND state = ?", request.ID, types.OpenAPIRequestStateWaiting).
			Updates(map[string]any{
				"state": types.OpenAPIRequestStateCancelled, "finished_at": time.Now().UTC(),
				"outcome_code": OutcomeWaitingCancelled.Code,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		return reconcileAndCleanupState(tx, request, manager.config.MaxTerminalRequests)
	})
}

func outcomeAllowsImmediateRelease(outcome Outcome) bool {
	return outcome.BusinessResult || !outcome.UpstreamAttempted
}

func randomSecretPair() (string, string, error) {
	requestBytes := make([]byte, 32)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(requestBytes); err != nil {
		return "", "", err
	}
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", err
	}
	return hex.EncodeToString(requestBytes), hex.EncodeToString(tokenBytes), nil
}

func hashReleaseToken(token string) string {
	return hashText(token)
}

func minDuration(left, right time.Duration) time.Duration {
	if left < right {
		return left
	}
	return right
}
