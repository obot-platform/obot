package executionbudget

import (
	"errors"
)

var (
	ErrInvalidConfiguration       = errors.New("openapi budget invalid configuration")
	ErrBudgetIdentityUnavailable  = errors.New("budget_identity_unavailable")
	ErrQueueFull                  = errors.New("budget_queue_full")
	ErrWaitDeadlineExceeded       = errors.New("budget_wait_deadline_exceeded")
	ErrLeaseNotFound              = errors.New("budget_lease_not_found")
	ErrInvalidReleaseToken        = errors.New("budget_invalid_release_token")
	ErrUncertainOutcome           = errors.New("budget_uncertain_outcome")
	ErrStorageUnavailable         = errors.New("budget_storage_unavailable")
	ErrRequestIDConflict          = errors.New("budget_request_id_conflict")
	ErrLimitExceeded              = errors.New("budget_limit_exceeded")
	ErrMCPContentTooLarge         = errors.New("mcp_content_too_large")
	ErrUnsupportedContentEncoding = errors.New("upstream_content_encoding_unsupported")
)
