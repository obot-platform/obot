package executionbudget

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

type BudgetContext struct {
	LeaseID        string        `json:"leaseID"`
	LeaseIssuedAt  time.Time     `json:"leaseIssuedAtUnixMs"`
	LeaseDeadline  time.Time     `json:"leaseDeadlineUnixMs"`
	FenceDeadline  time.Time     `json:"fenceDeadlineUnixMs"`
	DispatchedAt   time.Time     `json:"dispatchedAtUnixMs"`
	BudgetKeyHash  string        `json:"budgetKeyHash"`
	ClockSkewLimit time.Duration `json:"clockSkewLimitMs"`
	FencingMargin  time.Duration `json:"fencingMarginMs"`
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
	ResponseBytes     int64
	Retryable         bool
}

var (
	OutcomeWaitingCancelled        = Outcome{Code: "waiting_cancelled"}
	OutcomePreUpstreamCancelled    = Outcome{Code: "pre_upstream_cancelled", Retryable: true}
	OutcomePreUpstreamRejected     = Outcome{Code: "pre_upstream_rejected"}
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

type Manager struct {
	db     *gorm.DB
	config Config
	stop   chan struct{}
}

func NewManager(db *gorm.DB, config Config) (*Manager, error) {
	if db == nil {
		return nil, ErrStorageUnavailable
	}
	if db.Dialector.Name() != "postgres" {
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
			LeaseID:        request.RequestID,
			LeaseIssuedAt:  request.AdmittedAt.UTC(),
			LeaseDeadline:  request.Deadline.UTC(),
			FenceDeadline:  request.Deadline.Add(-manager.config.FencingMargin).UTC(),
			DispatchedAt:   time.Now().UTC(),
			BudgetKeyHash:  request.BudgetKeyHash,
			ClockSkewLimit: manager.config.FencingMargin / 2,
			FencingMargin:  manager.config.FencingMargin,
		},
		ReleaseToken: token,
	}, request, nil
}

func (manager *Manager) Release(ctx context.Context, leaseID, releaseToken string, outcome Outcome) error {
	if !outcomeAllowsImmediateRelease(outcome) {
		return ErrUncertainOutcome
	}
	request, err := manager.lockedRequest(ctx, leaseID)
	if err != nil {
		return err
	}
	if request.ReleaseTokenHash != hashReleaseToken(releaseToken) {
		return ErrInvalidReleaseToken
	}
	if request.State != types.OpenAPIRequestStateActive {
		return nil
	}
	finishedAt := time.Now().UTC()
	state := types.OpenAPIRequestStateCompleted
	switch {
	case outcome.BusinessResult:
		state = types.OpenAPIRequestStateCompleted
	case outcome.Retryable:
		state = types.OpenAPIRequestStateCancelled
	default:
		state = types.OpenAPIRequestStateFailed
	}
	if err = manager.db.WithContext(ctx).Model(&types.OpenAPIExecutionRequest{}).
		Where("id = ? AND state = ?", request.ID, types.OpenAPIRequestStateActive).
		Updates(map[string]any{
			"state": state, "finished_at": finishedAt, "outcome_code": outcome.Code,
			"response_bytes": outcome.ResponseBytes, "retryable": outcome.Retryable,
		}).Error; err != nil {
		return err
	}
	return manager.reconcileAndCleanup(ctx, request)
}

func (manager *Manager) CancelWaiting(ctx context.Context, requestID, releaseToken string) error {
	request, err := manager.lockedRequest(ctx, requestID)
	if err != nil {
		return err
	}
	if request.ReleaseTokenHash != hashReleaseToken(releaseToken) {
		return ErrInvalidReleaseToken
	}
	if request.State != types.OpenAPIRequestStateWaiting {
		return nil
	}
	if err = manager.db.WithContext(ctx).Model(&types.OpenAPIExecutionRequest{}).
		Where("id = ? AND state = ?", request.ID, types.OpenAPIRequestStateWaiting).
		Updates(map[string]any{
			"state": types.OpenAPIRequestStateCancelled, "finished_at": time.Now().UTC(),
			"outcome_code": OutcomeWaitingCancelled.Code,
		}).Error; err != nil {
		return err
	}
	return manager.reconcileAndCleanup(ctx, request)
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
