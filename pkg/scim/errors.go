package scim

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/pkg/scim/adapter"
)

const (
	scimTypeInvalidFilter = "invalidFilter"
	scimTypeInvalidPath   = "invalidPath"
	scimTypeInvalidSyntax = "invalidSyntax"
	scimTypeInvalidValue  = "invalidValue"
	scimTypeMutability    = "mutability"
	scimTypeNoTarget      = "noTarget"
	scimTypeTooMany       = "tooMany"
	scimTypeUniqueness    = "uniqueness"

	hintRetry                hint = "retry"
	hintUserDelete           hint = "userDelete"
	hintValuelessFilteredAdd hint = "valuelessFilteredAdd"
	hintExternalIDChanged    hint = "externalIDChanged"
)

// hint names a failure whose detail the connection's adapter.ErrorHints can add to.
type hint string

// Error is a SCIM error response, as RFC 7644 section 3.12 defines it.
type Error struct {
	Status   int
	ScimType string
	Detail   string
	// hint names the failure, so that the response carries the adapter's hint for it. The zero value has no hint.
	hint hint
}

type errorResponse struct {
	Schemas  []string `json:"schemas"`
	Status   string   `json:"status"`
	ScimType string   `json:"scimType,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

func (e *Error) Error() string {
	if e.ScimType != "" {
		return fmt.Sprintf("%d %s: %s", e.Status, e.ScimType, e.Detail)
	}
	return fmt.Sprintf("%d: %s", e.Status, e.Detail)
}

func (e *Error) response() errorResponse {
	return errorResponse{
		Schemas:  []string{errorSchema},
		Status:   strconv.Itoa(e.Status),
		ScimType: e.ScimType,
		Detail:   e.Detail,
	}
}

// withHints returns the error with the hint that hints have for its failure, if any. The error itself is not changed.
func (e *Error) withHints(hints adapter.ErrorHints) *Error {
	// UserDelete replaces the detail; the other hints are appended to it.
	var text string
	switch e.hint {
	case hintRetry:
		text = hints.Retry
	case hintUserDelete:
		if hints.UserDelete != "" {
			hinted := *e
			hinted.Detail = hints.UserDelete
			return &hinted
		}
	case hintValuelessFilteredAdd:
		text = hints.ValuelessFilteredAdd
	case hintExternalIDChanged:
		text = hints.ExternalIDChanged
	}
	if text == "" {
		return e
	}

	hinted := *e
	// Shared details are clauses without a final period, and hints are sentences.
	hinted.Detail = strings.TrimSuffix(e.Detail, ".") + ". " + text
	return &hinted
}

func badRequest(scimType, format string, args ...any) *Error {
	return &Error{
		Status:   http.StatusBadRequest,
		ScimType: scimType,
		Detail:   fmt.Sprintf(format, args...),
	}
}

func notFound(format string, args ...any) *Error {
	return &Error{
		Status: http.StatusNotFound,
		Detail: fmt.Sprintf(format, args...),
	}
}

func conflict(detail string) *Error {
	return &Error{
		Status:   http.StatusConflict,
		ScimType: scimTypeUniqueness,
		Detail:   detail,
	}
}
