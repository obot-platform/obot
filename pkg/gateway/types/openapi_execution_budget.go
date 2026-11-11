package types

import "time"

const (
	OpenAPIBudgetScopePlatform  = "platform"
	OpenAPIBudgetScopePool      = "pool"
	OpenAPIBudgetScopeOperation = "operation"

	OpenAPIBudgetPoolDefault   = "default"
	OpenAPIBudgetPoolDedicated = "dedicated"

	OpenAPIRequestStateWaiting   = "waiting"
	OpenAPIRequestStateActive    = "active"
	OpenAPIRequestStateCompleted = "completed"
	OpenAPIRequestStateCancelled = "cancelled"
	OpenAPIRequestStateExpired   = "expired"
	OpenAPIRequestStateFailed    = "failed"
)

type OpenAPIBudgetLimit struct {
	ID            uint      `gorm:"primaryKey"`
	Scope         string    `gorm:"uniqueIndex:idx_openapi_budget_limits_scope_key;size:32;not null"`
	KeyHash       string    `gorm:"uniqueIndex:idx_openapi_budget_limits_scope_key;size:64;not null;default:''"`
	MaxInFlight   int       `gorm:"not null"`
	MaxWaiting    int       `gorm:"not null"`
	OperationHash string    `gorm:"-"`
	CreatedAt     time.Time `gorm:"index"`
	UpdatedAt     time.Time
}

type OpenAPIOperationBudget struct {
	ID               uint      `gorm:"primaryKey"`
	BudgetSourceHash string    `gorm:"uniqueIndex:idx_openapi_operation_budgets_source_operation;size:64;not null"`
	OperationKeyHash string    `gorm:"uniqueIndex:idx_openapi_operation_budgets_source_operation;size:64;not null"`
	Pool             string    `gorm:"size:32;not null;default:dedicated;index:idx_openapi_operation_budgets_pool_updated,priority:1"`
	MaxInFlight      int       `gorm:"not null;default:4"`
	MaxWaiting       int       `gorm:"not null;default:8"`
	Version          uint      `gorm:"not null;default:1"`
	CreatedAt        time.Time `gorm:"index"`
	UpdatedAt        time.Time `gorm:"index:idx_openapi_operation_budgets_pool_updated,priority:2"`
}

type OpenAPIBudgetState struct {
	ID              uint      `gorm:"primaryKey"`
	Scope           string    `gorm:"uniqueIndex:idx_openapi_budget_states_scope_key;size:32;not null"`
	KeyHash         string    `gorm:"uniqueIndex:idx_openapi_budget_states_scope_key;size:64;not null"`
	ActiveCount     int       `gorm:"not null;default:0"`
	WaitingCount    int       `gorm:"not null;default:0"`
	LastAdmittedAt  time.Time `gorm:"not null"`
	ObotInstanceID  string    `gorm:"size:64;not null;default:''"`
	Version         uint      `gorm:"not null;default:1"`
	CreatedAt       time.Time `gorm:"index"`
	UpdatedAt       time.Time `gorm:"index"`
	OperationBudget bool      `gorm:"-"`
}

type OpenAPIExecutionRequest struct {
	ID               uint       `gorm:"primaryKey"`
	RequestID        string     `gorm:"uniqueIndex;size:64;not null"`
	BudgetSourceHash string     `gorm:"size:64;not null;index:idx_openapi_requests_budget_state_id,priority:1"`
	OperationKeyHash string     `gorm:"size:64;not null"`
	BudgetKeyHash    string     `gorm:"size:64;not null;index:idx_openapi_requests_budget_state_id,priority:1"`
	Pool             string     `gorm:"size:32;not null;index:idx_openapi_requests_pool_state_id,priority:1"`
	State            string     `gorm:"size:32;not null;index:idx_openapi_requests_state_deadline;index:idx_openapi_requests_state_id;index:idx_openapi_requests_budget_state_id,priority:2;index:idx_openapi_requests_pool_state_id,priority:2"`
	EnqueuedAt       time.Time  `gorm:"not null;index"`
	WaitDeadline     time.Time  `gorm:"not null"`
	Deadline         time.Time  `gorm:"not null;index:idx_openapi_requests_state_deadline"`
	AdmittedAt       *time.Time `gorm:"index"`
	FinishedAt       *time.Time `gorm:"index"`
	ReleaseTokenHash string     `gorm:"size:64;not null"`
	LeaseGeneration  uint       `gorm:"not null;default:0"`
	ObotInstanceID   string     `gorm:"size:64;not null;default:''"`
	WorkerInstanceID *string    `gorm:"size:64"`
	OutcomeCode      string     `gorm:"size:64;not null;default:''"`
	ResponseBytes    int64      `gorm:"not null;default:0"`
	Retryable        bool       `gorm:"not null;default:false"`
	LastErrorCode    string     `gorm:"size:64;not null;default:''"`
	CreatedAt        time.Time  `gorm:"index"`
	UpdatedAt        time.Time  `gorm:"index"`
}

func (OpenAPIBudgetLimit) TableName() string { return "openapi_budget_limits" }

func (OpenAPIOperationBudget) TableName() string { return "openapi_operation_budgets" }

func (OpenAPIBudgetState) TableName() string { return "openapi_budget_states" }

func (OpenAPIExecutionRequest) TableName() string { return "openapi_execution_requests" }
