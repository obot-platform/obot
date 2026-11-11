package executionbudget

import (
	"context"
	"errors"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (manager *Manager) ReclaimExpired(ctx context.Context) (int, error) {
	now := time.Now().UTC()
	var requests []types.OpenAPIExecutionRequest
	reclaimed := 0
	err := manager.transaction(ctx, func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where(
				"(state = ? AND wait_deadline < ?) OR (state = ? AND deadline < ?)",
				types.OpenAPIRequestStateWaiting, now.Add(-manager.config.ReclaimGrace),
				types.OpenAPIRequestStateActive, now.Add(-manager.config.ReclaimGrace),
			).Limit(500).Find(&requests).Error; err != nil {
			return err
		}
		for _, request := range requests {
			newState := types.OpenAPIRequestStateExpired
			outcome := OutcomeObotLost.Code
			if request.State == types.OpenAPIRequestStateWaiting {
				outcome = OutcomeWaitingCancelled.Code
			}
			result := tx.Model(&types.OpenAPIExecutionRequest{}).
				Where("id = ? AND state = ?", request.ID, request.State).
				Updates(map[string]any{"state": newState, "finished_at": now, "outcome_code": outcome})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			reclaimed++
			if err := reconcileScopeCounts(tx, []scopeKey{
				{Scope: types.OpenAPIBudgetScopePlatform, KeyHash: ""},
				{Scope: types.OpenAPIBudgetScopePool, KeyHash: hashText(request.Pool)},
				{Scope: types.OpenAPIBudgetScopeOperation, KeyHash: request.BudgetKeyHash},
			}); err != nil {
				return err
			}
			var nonterminal int64
			if err := tx.Model(&types.OpenAPIExecutionRequest{}).
				Where("budget_key_hash = ? AND state IN ?", request.BudgetKeyHash, []string{
					types.OpenAPIRequestStateWaiting, types.OpenAPIRequestStateActive,
				}).Count(&nonterminal).Error; err != nil {
				return err
			}
			if nonterminal == 0 {
				if err := tx.Where("scope = ? AND key_hash = ?", types.OpenAPIBudgetScopeOperation, request.BudgetKeyHash).
					Delete(&types.OpenAPIBudgetState{}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	return reclaimed, err
}

func (manager *Manager) CleanupTerminal(ctx context.Context) (int64, error) {
	result := manager.db.WithContext(ctx).
		Where("state IN ? AND finished_at < ?", []string{
			types.OpenAPIRequestStateCompleted, types.OpenAPIRequestStateCancelled,
			types.OpenAPIRequestStateExpired, types.OpenAPIRequestStateFailed,
		}, time.Now().UTC().Add(-manager.config.TerminalRetention)).
		Delete(&types.OpenAPIExecutionRequest{})
	return result.RowsAffected, result.Error
}

func (manager *Manager) reconcileAndCleanup(ctx context.Context, request types.OpenAPIExecutionRequest) error {
	return manager.transaction(ctx, func(tx *gorm.DB) error {
		if err := reconcileScopeCounts(tx, []scopeKey{
			{Scope: types.OpenAPIBudgetScopePlatform, KeyHash: ""},
			{Scope: types.OpenAPIBudgetScopePool, KeyHash: hashText(request.Pool)},
			{Scope: types.OpenAPIBudgetScopeOperation, KeyHash: request.BudgetKeyHash},
		}); err != nil {
			return err
		}
		var nonterminal int64
		if err := tx.Model(&types.OpenAPIExecutionRequest{}).
			Where("budget_key_hash = ? AND state IN ?", request.BudgetKeyHash, []string{
				types.OpenAPIRequestStateWaiting, types.OpenAPIRequestStateActive,
			}).Count(&nonterminal).Error; err != nil {
			return err
		}
		if nonterminal == 0 {
			if err := tx.Where("scope = ? AND key_hash = ?", types.OpenAPIBudgetScopeOperation, request.BudgetKeyHash).
				Delete(&types.OpenAPIBudgetState{}).Error; err != nil {
				return err
			}
		}
		var terminal int64
		if err := tx.Model(&types.OpenAPIExecutionRequest{}).Where("state IN ?", []string{
			types.OpenAPIRequestStateCompleted, types.OpenAPIRequestStateCancelled,
			types.OpenAPIRequestStateExpired, types.OpenAPIRequestStateFailed,
		}).Count(&terminal).Error; err != nil {
			return err
		}
		if terminal > manager.config.MaxTerminalRequests {
			if err := tx.Where("id IN (?)", tx.Model(&types.OpenAPIExecutionRequest{}).
				Select("id").Where("state IN ?", []string{
				types.OpenAPIRequestStateCompleted, types.OpenAPIRequestStateCancelled,
				types.OpenAPIRequestStateExpired, types.OpenAPIRequestStateFailed,
			}).Order("finished_at, id").Limit(int(terminal-manager.config.MaxTerminalRequests))).Delete(&types.OpenAPIExecutionRequest{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (manager *Manager) ReconcileAll(ctx context.Context) error {
	var states []types.OpenAPIBudgetState
	if err := manager.db.WithContext(ctx).Find(&states).Error; err != nil {
		return err
	}
	scopes := make([]scopeKey, 0, len(states))
	for _, state := range states {
		scopes = append(scopes, scopeKey{Scope: state.Scope, KeyHash: state.KeyHash})
	}
	return manager.transaction(ctx, func(tx *gorm.DB) error {
		if err := reconcileScopeCounts(tx, scopes); err != nil {
			return err
		}
		var operations []types.OpenAPIBudgetState
		if err := tx.Where("scope = ?", types.OpenAPIBudgetScopeOperation).Find(&operations).Error; err != nil {
			return err
		}
		for _, operation := range operations {
			var nonterminal int64
			if err := tx.Model(&types.OpenAPIExecutionRequest{}).
				Where("budget_key_hash = ? AND state IN ?", operation.KeyHash, []string{
					types.OpenAPIRequestStateWaiting, types.OpenAPIRequestStateActive,
				}).Count(&nonterminal).Error; err != nil {
				return err
			}
			if nonterminal == 0 {
				if err := tx.Where("id = ?", operation.ID).Delete(&types.OpenAPIBudgetState{}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func IsNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
