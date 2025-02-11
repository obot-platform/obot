package types

import (
	"errors"
	"fmt"
	"net/http"
)

type ErrHTTP struct {
	Code    int
	Message string
}

func (e *ErrHTTP) Error() string {
	return fmt.Sprintf("error code %d (%s): %s", e.Code, http.StatusText(e.Code), e.Message)
}

func NewErrHTTP(code int, message string) *ErrHTTP {
	return &ErrHTTP{
		Code:    code,
		Message: message,
	}
}

func NewErrBadRequest(message string, args ...interface{}) *ErrHTTP {
	return NewErrHTTP(http.StatusBadRequest, fmt.Sprintf(message, args...))
}

func NewErrNotFound(message string, args ...any) *ErrHTTP {
	if message == "" {
		message = "not found"
	}
	if len(args) > 0 {
		message = fmt.Sprintf(message, args...)
	}
	return NewErrHTTP(http.StatusNotFound, message)
}

func IsNotFound(err error) bool {
	var errHTTP *ErrHTTP
	return errors.As(err, &errHTTP) && errHTTP.Code == http.StatusNotFound
}
