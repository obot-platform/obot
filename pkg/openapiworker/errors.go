package openapiworker

import (
	"errors"
	"fmt"
	"net/http"
)

type ErrorCode string

const (
	ErrorCodePlatformUnauthenticated       ErrorCode = "platform_unauthenticated"
	ErrorCodePlatformForbidden             ErrorCode = "platform_forbidden"
	ErrorCodeResourceHidden                ErrorCode = "resource_hidden"
	ErrorCodeOperationNotFound             ErrorCode = "operation_not_found"
	ErrorCodeBusinessAuthorizationRequired ErrorCode = "business_authorization_required"
	ErrorCodeInvalidRequest                ErrorCode = "invalid_request"
	ErrorCodeUnsupportedMediaType          ErrorCode = "unsupported_media_type"
	ErrorCodeRequestTooLarge               ErrorCode = "request_too_large"
	ErrorCodeBudgetRejected                ErrorCode = "budget_rejected"
	ErrorCodeBudgetIdentityUnavailable     ErrorCode = "budget_identity_unavailable"
	ErrorCodeWorkerUnavailable             ErrorCode = "worker_unavailable"
	ErrorCodeProtocolError                 ErrorCode = "protocol_error"
	ErrorCodeDeadlineExceeded              ErrorCode = "deadline_exceeded"
	ErrorCodeUpstreamProtocolUnsupported   ErrorCode = "upstream_protocol_unsupported"
	ErrorCodeResponseTooLarge              ErrorCode = "response_too_large"
	ErrorCodeTransportUncertain            ErrorCode = "transport_uncertain"
	ErrorCodeSnapshotRequired              ErrorCode = "snapshot_required"
	ErrorCodeSnapshotInvalid               ErrorCode = "snapshot_invalid"
	ErrorCodeAmbiguousOperation            ErrorCode = "ambiguous_operation"
)

type Error struct {
	Code       ErrorCode `json:"code"`
	Message    string    `json:"message"`
	RequestID  string    `json:"requestID,omitempty"`
	Retryable  bool      `json:"retryable"`
	HTTPStatus int       `json:"-"`
}

func (e *Error) Error() string {
	if e.RequestID == "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s (request %s)", e.Code, e.Message, e.RequestID)
}

func NewError(code ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message, HTTPStatus: errorHTTPStatus(code)}
}

func AsError(err error) (*Error, bool) {
	var workerErr *Error
	return workerErr, errors.As(err, &workerErr)
}

func errorHTTPStatus(code ErrorCode) int {
	switch code {
	case ErrorCodePlatformUnauthenticated:
		return http.StatusUnauthorized
	case ErrorCodePlatformForbidden, ErrorCodeResourceHidden:
		return http.StatusForbidden
	case ErrorCodeOperationNotFound:
		return http.StatusNotFound
	case ErrorCodeBusinessAuthorizationRequired:
		return http.StatusConflict
	case ErrorCodeRequestTooLarge:
		return http.StatusRequestEntityTooLarge
	case ErrorCodeBudgetRejected:
		return http.StatusTooManyRequests
	case ErrorCodeBudgetIdentityUnavailable:
		return http.StatusServiceUnavailable
	case ErrorCodeWorkerUnavailable:
		return http.StatusServiceUnavailable
	case ErrorCodeDeadlineExceeded:
		return http.StatusGatewayTimeout
	case ErrorCodeUnsupportedMediaType, ErrorCodeUpstreamProtocolUnsupported, ErrorCodeResponseTooLarge:
		return http.StatusBadGateway
	default:
		return http.StatusBadRequest
	}
}
