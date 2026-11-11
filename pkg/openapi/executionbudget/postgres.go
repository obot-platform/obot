package executionbudget

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type scopeKey struct {
	Scope   string
	KeyHash string
}

func (manager *Manager) enqueue(ctx context.Context, request AcquireRequest, requestID, releaseToken string, waitDeadline time.Time) (types.OpenAPIExecutionRequest, error) {
	var result types.OpenAPIExecutionRequest
	err := manager.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&types.OpenAPIBudgetState{
			Scope: types.OpenAPIBudgetScopeOperation, KeyHash: request.Identity.BudgetKeyHash,
			LastAdmittedAt: time.Now().UTC(), ObotInstanceID: request.ObotInstanceID,
		}).Error; err != nil {
			return err
		}
		if err := lockBudgetScopes(tx, request.Identity.BudgetKeyHash); err != nil {
			return err
		}
		pool, operationLimit, err := manager.operationBudget(tx, request.Identity)
		if err != nil {
			return err
		}
		if err := manager.reconcileCounts(tx, request.Identity.BudgetKeyHash, pool); err != nil {
			return err
		}
		platform, _, operationState, err := lockAndLoadStates(tx, request.Identity.BudgetKeyHash, pool)
		if err != nil {
			return err
		}
		var existing types.OpenAPIExecutionRequest
		if err = tx.Where("request_id = ?", requestID).Take(&existing).Error; err == nil {
			return ErrRequestIDConflict
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if platform.WaitingCount >= manager.config.PlatformMaxWaiting {
			return ErrQueueFull
		}
		if operationState.WaitingCount >= operationLimit.MaxWaiting {
			return ErrQueueFull
		}
		now := time.Now().UTC()
		result = types.OpenAPIExecutionRequest{
			RequestID: requestID, BudgetSourceHash: request.Identity.BudgetSourceHash,
			OperationKeyHash: request.Identity.OperationKeyHash, BudgetKeyHash: request.Identity.BudgetKeyHash,
			Pool: pool, State: types.OpenAPIRequestStateWaiting, EnqueuedAt: now, WaitDeadline: waitDeadline,
			Deadline: request.Deadline.UTC(), ReleaseTokenHash: hashReleaseToken(releaseToken),
			ObotInstanceID: request.ObotInstanceID,
		}
		if err = tx.Create(&result).Error; err != nil {
			return err
		}
		return tx.Model(&types.OpenAPIBudgetState{}).Where("id = ?", operationState.ID).
			UpdateColumn("waiting_count", gorm.Expr("waiting_count + 1")).Error
	})
	if err != nil {
		return types.OpenAPIExecutionRequest{}, err
	}
	return result, nil
}

func (manager *Manager) AdmitOne(ctx context.Context) (bool, error) {
	manager.admitMu.Lock()
	defer manager.admitMu.Unlock()
	candidates, err := manager.runtimeCandidates(ctx)
	if err != nil || len(candidates) == 0 {
		return false, err
	}
	admitted := false
	err = manager.transaction(ctx, func(tx *gorm.DB) error {
		if err := lockPlatform(tx); err != nil {
			return err
		}
		if err := lockPools(tx); err != nil {
			return err
		}
		if err := lockOperations(tx, candidates); err != nil {
			return err
		}
		for _, candidate := range candidates {
			identity := BudgetIdentity{BudgetKeyHash: candidate.KeyHash}
			pool, operationLimit, err := manager.operationBudget(tx, identity)
			if err != nil {
				return err
			}
			if err := manager.reconcileCounts(tx, candidate.KeyHash, pool); err != nil {
				return err
			}
			platform, poolState, operationState, err := lockAndLoadStates(tx, candidate.KeyHash, pool)
			if err != nil {
				return err
			}
			if platform.ActiveCount >= manager.config.PlatformMaxInFlight ||
				poolState.ActiveCount >= manager.poolLimit(pool) ||
				operationState.ActiveCount >= operationLimit.MaxInFlight {
				continue
			}
			var waiting types.OpenAPIExecutionRequest
			err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("budget_key_hash = ? AND state = ? AND wait_deadline > ?", candidate.KeyHash, types.OpenAPIRequestStateWaiting, time.Now().UTC()).
				Order("id").First(&waiting).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			if err = tx.Model(&types.OpenAPIExecutionRequest{}).
				Where("id = ? AND state = ?", waiting.ID, types.OpenAPIRequestStateWaiting).
				Updates(map[string]any{
					"state": types.OpenAPIRequestStateActive, "admitted_at": now,
					"lease_generation": gorm.Expr("lease_generation + 1"),
				}).Error; err != nil {
				return err
			}
			if err = tx.Model(&types.OpenAPIBudgetState{}).Where("id = ?", operationState.ID).Updates(map[string]any{
				"active_count":     gorm.Expr("active_count + 1"),
				"waiting_count":    gorm.Expr("GREATEST(waiting_count - 1, 0)"),
				"last_admitted_at": now,
				"version":          gorm.Expr("version + 1"),
			}).Error; err != nil {
				return err
			}
			admitted = true
			return nil
		}
		return gorm.ErrRecordNotFound
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	return admitted, err
}

func (manager *Manager) runtimeCandidates(ctx context.Context) ([]types.OpenAPIBudgetState, error) {
	var states []types.OpenAPIBudgetState
	err := manager.db.WithContext(ctx).
		Where("scope = ? AND waiting_count > 0", types.OpenAPIBudgetScopeOperation).
		Order("last_admitted_at, id").Limit(manager.config.PlatformMaxWaiting).Find(&states).Error
	return states, err
}

func (manager *Manager) operationBudget(tx *gorm.DB, identity BudgetIdentity) (string, types.OpenAPIOperationBudget, error) {
	var configured types.OpenAPIOperationBudget
	err := tx.Where("budget_source_hash = ? AND operation_key_hash = ?", identity.BudgetSourceHash, identity.OperationKeyHash).
		Take(&configured).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.OpenAPIBudgetPoolDefault, types.OpenAPIOperationBudget{
			Pool: types.OpenAPIBudgetPoolDefault, MaxInFlight: manager.config.DefaultOperationInFlight,
			MaxWaiting: manager.config.DefaultOperationWaiting,
		}, nil
	}
	if err != nil {
		return "", configured, err
	}
	return configured.Pool, configured, nil
}

func (manager *Manager) poolLimit(pool string) int {
	if pool == types.OpenAPIBudgetPoolDedicated {
		return manager.config.DedicatedPoolMaxInFlight
	}
	return manager.config.DefaultPoolMaxInFlight
}

func (manager *Manager) transaction(ctx context.Context, operation func(*gorm.DB) error) error {
	transactionCtx, cancel := context.WithTimeout(ctx, manager.config.TransactionTimeout)
	defer cancel()
	tx := manager.db.WithContext(transactionCtx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, tx.Error)
	}
	if err := operation(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("%w: %v", ErrStorageUnavailable, err)
	}
	return nil
}

func lockedRequest(tx *gorm.DB, requestID string) (types.OpenAPIExecutionRequest, error) {
	var request types.OpenAPIExecutionRequest
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("request_id = ?", requestID).First(&request).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return request, ErrLeaseNotFound
	}
	return request, err
}

func lockBudgetScopes(tx *gorm.DB, operationHash string) error {
	if err := lockPlatform(tx); err != nil {
		return err
	}
	if err := lockPools(tx); err != nil {
		return err
	}
	return lockOperations(tx, []types.OpenAPIBudgetState{{KeyHash: operationHash}})
}

func lockPlatform(tx *gorm.DB) error {
	var state types.OpenAPIBudgetState
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("scope = ? AND key_hash = ''", types.OpenAPIBudgetScopePlatform).First(&state).Error
}

func lockPools(tx *gorm.DB) error {
	var states []types.OpenAPIBudgetState
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("scope = ?", types.OpenAPIBudgetScopePool).Order("key_hash").Find(&states).Error
}

func lockOperations(tx *gorm.DB, candidates []types.OpenAPIBudgetState) error {
	hashes := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		hashes = append(hashes, candidate.KeyHash)
	}
	var states []types.OpenAPIBudgetState
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("scope = ? AND key_hash IN ?", types.OpenAPIBudgetScopeOperation, hashes).
		Order("last_admitted_at, id").Find(&states).Error
}

func lockAndLoadStates(tx *gorm.DB, operationHash, pool string) (types.OpenAPIBudgetState, types.OpenAPIBudgetState, types.OpenAPIBudgetState, error) {
	var platform, poolState, operationState types.OpenAPIBudgetState
	load := func(destination *types.OpenAPIBudgetState, scope, key string) error {
		return tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("scope = ? AND key_hash = ?", scope, key).First(destination).Error
	}
	if err := load(&platform, types.OpenAPIBudgetScopePlatform, ""); err != nil {
		return platform, poolState, operationState, err
	}
	if err := load(&poolState, types.OpenAPIBudgetScopePool, hashText(pool)); err != nil {
		return platform, poolState, operationState, err
	}
	if err := load(&operationState, types.OpenAPIBudgetScopeOperation, operationHash); err != nil {
		return platform, poolState, operationState, err
	}
	return platform, poolState, operationState, nil
}

func (manager *Manager) reconcileCounts(tx *gorm.DB, operationHash, pool string) error {
	return reconcileScopeCounts(tx, []scopeKey{
		{Scope: types.OpenAPIBudgetScopePlatform, KeyHash: ""},
		{Scope: types.OpenAPIBudgetScopePool, KeyHash: hashText(pool)},
		{Scope: types.OpenAPIBudgetScopeOperation, KeyHash: operationHash},
	})
}

func reconcileScopeCounts(tx *gorm.DB, scopes []scopeKey) error {
	for _, scope := range scopes {
		var active, waiting int64
		if err := countRequests(tx, scope, types.OpenAPIRequestStateActive, &active); err != nil {
			return err
		}
		if err := countRequests(tx, scope, types.OpenAPIRequestStateWaiting, &waiting); err != nil {
			return err
		}
		if err := tx.Model(&types.OpenAPIBudgetState{}).
			Where("scope = ? AND key_hash = ?", scope.Scope, scope.KeyHash).
			Updates(map[string]any{"active_count": active, "waiting_count": waiting}).Error; err != nil {
			return err
		}
	}
	return nil
}

func countRequests(tx *gorm.DB, scope scopeKey, state string, destination *int64) error {
	query := tx.Model(&types.OpenAPIExecutionRequest{}).Where("state = ?", state)
	switch scope.Scope {
	case types.OpenAPIBudgetScopePlatform:
	case types.OpenAPIBudgetScopePool:
		poolName := poolFromHash(scope.KeyHash)
		query = query.Where("pool = ?", poolName)
	case types.OpenAPIBudgetScopeOperation:
		query = query.Where("budget_key_hash = ?", scope.KeyHash)
	}
	return query.Count(destination).Error
}

func poolFromHash(value string) string {
	if value == hashText(types.OpenAPIBudgetPoolDedicated) {
		return types.OpenAPIBudgetPoolDedicated
	}
	return types.OpenAPIBudgetPoolDefault
}

func hashText(value string) string { return hash.String(value) }
