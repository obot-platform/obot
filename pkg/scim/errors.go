package scim

import (
	"fmt"
	"net/http"
	"strconv"
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
)

// Error is a SCIM error response, as RFC 7644 section 3.12 defines it.
type Error struct {
	Status   int
	ScimType string
	Detail   string
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
