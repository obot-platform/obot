package types

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestErrHTTPError(t *testing.T) {
	tests := []struct {
		name     string
		err      *ErrHTTP
		expected string
	}{
		{
			name:     "400 Bad Request",
			err:      &ErrHTTP{Code: http.StatusBadRequest, Message: "Invalid input"},
			expected: "error code 400 (Bad Request): Invalid input",
		},
		{
			name:     "404 Not Found",
			err:      &ErrHTTP{Code: http.StatusNotFound, Message: "Resource not found"},
			expected: "error code 404 (Not Found): Resource not found",
		},
		{
			name:     "403 Forbidden",
			err:      &ErrHTTP{Code: http.StatusForbidden, Message: "Access denied"},
			expected: "error code 403 (Forbidden): Access denied",
		},
		{
			name:     "409 Conflict",
			err:      &ErrHTTP{Code: http.StatusConflict, Message: "Already exists"},
			expected: "error code 409 (Conflict): Already exists",
		},
		{
			name:     "500 Internal Server Error",
			err:      &ErrHTTP{Code: http.StatusInternalServerError, Message: "Server error"},
			expected: "error code 500 (Internal Server Error): Server error",
		},
		{
			name:     "Empty message",
			err:      &ErrHTTP{Code: http.StatusBadRequest, Message: ""},
			expected: "error code 400 (Bad Request): ",
		},
		{
			name:     "Unknown status code",
			err:      &ErrHTTP{Code: 999, Message: "Unknown"},
			expected: "error code 999 (): Unknown", // http.StatusText returns empty string for unknown codes
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("Error() = %q; want %q", result, tt.expected)
			}
		})
	}
}

func TestNewErrHTTP(t *testing.T) {
	err := NewErrHTTP(http.StatusBadRequest, "test message")

	if err.Code != http.StatusBadRequest {
		t.Errorf("NewErrHTTP() Code = %d; want %d", err.Code, http.StatusBadRequest)
	}
	if err.Message != "test message" {
		t.Errorf("NewErrHTTP() Message = %q; want %q", err.Message, "test message")
	}
}

func TestNewErrBadRequest(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		args         []any
		expectedCode int
		expectedMsg  string
	}{
		{
			name:         "simple message",
			message:      "Invalid input",
			args:         nil,
			expectedCode: http.StatusBadRequest,
			expectedMsg:  "Invalid input",
		},
		{
			name:         "formatted message",
			message:      "Field %s is required",
			args:         []any{"username"},
			expectedCode: http.StatusBadRequest,
			expectedMsg:  "Field username is required",
		},
		{
			name:         "multiple format args",
			message:      "Expected %d items, got %d",
			args:         []any{10, 5},
			expectedCode: http.StatusBadRequest,
			expectedMsg:  "Expected 10 items, got 5",
		},
		{
			name:         "empty message",
			message:      "",
			args:         nil,
			expectedCode: http.StatusBadRequest,
			expectedMsg:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewErrBadRequest(tt.message, tt.args...)

			if err.Code != tt.expectedCode {
				t.Errorf("NewErrBadRequest() Code = %d; want %d", err.Code, tt.expectedCode)
			}
			if err.Message != tt.expectedMsg {
				t.Errorf("NewErrBadRequest() Message = %q; want %q", err.Message, tt.expectedMsg)
			}
		})
	}
}

func TestNewErrNotFound(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		args         []any
		expectedCode int
		expectedMsg  string
	}{
		{
			name:         "empty message defaults to 'not found'",
			message:      "",
			args:         nil,
			expectedCode: http.StatusNotFound,
			expectedMsg:  "not found",
		},
		{
			name:         "simple message",
			message:      "User not found",
			args:         nil,
			expectedCode: http.StatusNotFound,
			expectedMsg:  "User not found",
		},
		{
			name:         "formatted message",
			message:      "Resource %s not found",
			args:         []any{"abc123"},
			expectedCode: http.StatusNotFound,
			expectedMsg:  "Resource abc123 not found",
		},
		{
			name:         "empty message with args still formats",
			message:      "",
			args:         []any{"ignored"},
			expectedCode: http.StatusNotFound,
			expectedMsg:  "not found%!(EXTRA string=ignored)", // fmt.Sprintf still tries to format even with empty string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewErrNotFound(tt.message, tt.args...)

			if err.Code != tt.expectedCode {
				t.Errorf("NewErrNotFound() Code = %d; want %d", err.Code, tt.expectedCode)
			}
			if err.Message != tt.expectedMsg {
				t.Errorf("NewErrNotFound() Message = %q; want %q", err.Message, tt.expectedMsg)
			}
		})
	}
}

func TestNewErrForbidden(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		args         []any
		expectedCode int
		expectedMsg  string
	}{
		{
			name:         "simple message",
			message:      "Access denied",
			args:         nil,
			expectedCode: http.StatusForbidden,
			expectedMsg:  "Access denied",
		},
		{
			name:         "formatted message",
			message:      "User %s does not have permission",
			args:         []any{"john"},
			expectedCode: http.StatusForbidden,
			expectedMsg:  "User john does not have permission",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewErrForbidden(tt.message, tt.args...)

			if err.Code != tt.expectedCode {
				t.Errorf("NewErrForbidden() Code = %d; want %d", err.Code, tt.expectedCode)
			}
			if err.Message != tt.expectedMsg {
				t.Errorf("NewErrForbidden() Message = %q; want %q", err.Message, tt.expectedMsg)
			}
		})
	}
}

func TestNewErrAlreadyExists(t *testing.T) {
	tests := []struct {
		name         string
		message      string
		args         []any
		expectedCode int
		expectedMsg  string
	}{
		{
			name:         "simple message",
			message:      "Resource already exists",
			args:         nil,
			expectedCode: http.StatusConflict,
			expectedMsg:  "Resource already exists",
		},
		{
			name:         "formatted message",
			message:      "User %s already exists",
			args:         []any{"john@example.com"},
			expectedCode: http.StatusConflict,
			expectedMsg:  "User john@example.com already exists",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewErrAlreadyExists(tt.message, tt.args...)

			if err.Code != tt.expectedCode {
				t.Errorf("NewErrAlreadyExists() Code = %d; want %d", err.Code, tt.expectedCode)
			}
			if err.Message != tt.expectedMsg {
				t.Errorf("NewErrAlreadyExists() Message = %q; want %q", err.Message, tt.expectedMsg)
			}
		})
	}
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "404 error",
			err:      NewErrNotFound("Not found"),
			expected: true,
		},
		{
			name:     "400 error",
			err:      NewErrBadRequest("Bad request"),
			expected: false,
		},
		{
			name:     "403 error",
			err:      NewErrForbidden("Forbidden"),
			expected: false,
		},
		{
			name:     "409 error",
			err:      NewErrAlreadyExists("Already exists"),
			expected: false,
		},
		{
			name:     "standard error",
			err:      errors.New("standard error"),
			expected: false,
		},
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "wrapped 404 error",
			err:      wrapError(NewErrNotFound("Not found")),
			expected: true,
		},
		{
			name:     "wrapped non-404 error",
			err:      wrapError(NewErrBadRequest("Bad request")),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNotFound(tt.err)
			if result != tt.expected {
				t.Errorf("IsNotFound() = %v; want %v", result, tt.expected)
			}
		})
	}
}

// Helper function to test error wrapping
func wrapError(err error) error {
	if err == nil {
		return nil
	}
	return &wrappedError{cause: err}
}

type wrappedError struct {
	cause error
}

func (w *wrappedError) Error() string {
	return "wrapped: " + w.cause.Error()
}

func (w *wrappedError) Unwrap() error {
	return w.cause
}

func TestErrHTTPImplementsError(t *testing.T) {
	var _ error = &ErrHTTP{}
	var _ error = (*ErrHTTP)(nil)
}

func TestErrHTTPErrorFormat(t *testing.T) {
	// Test that Error() output contains expected components
	err := NewErrHTTP(http.StatusBadRequest, "test message")
	errStr := err.Error()

	if !strings.Contains(errStr, "400") {
		t.Errorf("Error() output should contain status code 400, got: %q", errStr)
	}
	if !strings.Contains(errStr, "Bad Request") {
		t.Errorf("Error() output should contain status text 'Bad Request', got: %q", errStr)
	}
	if !strings.Contains(errStr, "test message") {
		t.Errorf("Error() output should contain message 'test message', got: %q", errStr)
	}
}
