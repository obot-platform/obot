package db

import (
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errUnsupportedBudgetDatabase = errors.New("OpenAPI budgets require PostgreSQL or explicit SQLite development mode")
)

func AutoMigrateOpenAPIBudgets(tx *gorm.DB) error {
	dialect := tx.Name()
	if dialect != "postgres" && dialect != "sqlite" {
		return errUnsupportedBudgetDatabase
	}
	if dialect == "sqlite" {
		if err := tx.AutoMigrate(
			&types.OpenAPIBudgetLimit{},
			&types.OpenAPIOperationBudget{},
			&types.OpenAPIBudgetState{},
			&types.OpenAPIExecutionRequest{},
		); err != nil {
			return fmt.Errorf("failed to migrate OpenAPI budget tables: %w", err)
		}
		return nil
	}
	return tx.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(8109202601110901)).Error; err != nil {
			return fmt.Errorf("failed to lock OpenAPI budget migration: %w", err)
		}
		return migratePostgresOpenAPIBudgets(tx)
	})
}

func migratePostgresOpenAPIBudgets(tx *gorm.DB) error {
	if err := tx.AutoMigrate(
		&types.OpenAPIBudgetLimit{},
		&types.OpenAPIOperationBudget{},
		&types.OpenAPIBudgetState{},
		&types.OpenAPIExecutionRequest{},
	); err != nil {
		return fmt.Errorf("failed to migrate OpenAPI budget tables: %w", err)
	}

	if tx.Name() != "postgres" {
		return nil
	}

	constraints := []struct {
		table, name, expression string
	}{
		{"openapi_budget_limits", "openapi_budget_limits_positive", "max_in_flight > 0 AND max_waiting > 0"},
		{"openapi_operation_budgets", "openapi_operation_budgets_pool", "pool IN ('default', 'dedicated')"},
		{"openapi_operation_budgets", "openapi_operation_budgets_positive", "max_in_flight > 0 AND max_in_flight <= 12 AND max_waiting > 0 AND max_waiting <= 16"},
		{"openapi_budget_states", "openapi_budget_states_nonnegative", "active_count >= 0 AND waiting_count >= 0"},
		{"openapi_execution_requests", "openapi_execution_requests_state", "state IN ('waiting','active','completed','cancelled','expired','failed')"},
		{"openapi_execution_requests", "openapi_execution_requests_deadlines", "wait_deadline <= deadline"},
	}
	for _, constraint := range constraints {
		if err := addOpenAPIBudgetConstraint(tx, constraint.table, constraint.name, constraint.expression); err != nil {
			return err
		}
	}

	defaults := []types.OpenAPIBudgetLimit{
		{Scope: types.OpenAPIBudgetScopePlatform, KeyHash: "", MaxInFlight: 32, MaxWaiting: 64},
		{Scope: types.OpenAPIBudgetScopePool, KeyHash: hashText(types.OpenAPIBudgetPoolDefault), MaxInFlight: 16, MaxWaiting: 64},
		{Scope: types.OpenAPIBudgetScopePool, KeyHash: hashText(types.OpenAPIBudgetPoolDedicated), MaxInFlight: 16, MaxWaiting: 64},
	}
	for _, limit := range defaults {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "scope"}, {Name: "key_hash"}},
			DoNothing: true,
		}).Create(&limit).Error; err != nil {
			return fmt.Errorf("failed to seed OpenAPI budget defaults: %w", err)
		}
	}
	now := time.Now().UTC()
	runtimeStates := []types.OpenAPIBudgetState{
		{Scope: types.OpenAPIBudgetScopePlatform, KeyHash: "", LastAdmittedAt: now},
		{Scope: types.OpenAPIBudgetScopePool, KeyHash: hashText(types.OpenAPIBudgetPoolDefault), LastAdmittedAt: now},
		{Scope: types.OpenAPIBudgetScopePool, KeyHash: hashText(types.OpenAPIBudgetPoolDedicated), LastAdmittedAt: now},
	}
	for _, state := range runtimeStates {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "scope"}, {Name: "key_hash"}},
			DoNothing: true,
		}).Create(&state).Error; err != nil {
			return fmt.Errorf("failed to seed OpenAPI budget runtime states: %w", err)
		}
	}
	return nil
}

func addOpenAPIBudgetConstraint(tx *gorm.DB, table, name, expression string) error {
	var count int64
	if err := tx.Raw(`
		SELECT COUNT(*)
		FROM pg_constraint
		WHERE conname = ? AND conrelid = to_regclass(?)`,
		name, table,
	).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return tx.Exec(fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s)", table, name, expression)).Error
}

func hashText(value string) string {
	return hash.String(value)
}
