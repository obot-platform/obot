package ratelimit

import (
	"net/http"

	"k8s.io/apiserver/pkg/authentication/user"
)

// NoopMiddleware is a no-operation implementation of the Middleware interface.
// It doesn't perform any actual rate limiting, making it useful for development
// environments or when rate limiting needs to be temporarily disabled.
type NoopMiddleware struct{}

// Limit implements the Middleware interface for NoopMiddleware.
// It simply passes requests through without any rate limiting.
func (NoopMiddleware) Limit(next http.Handler) http.Handler {
	return next
}

// RegisterUser implements the Middleware interface for NoopMiddleware.
// It does nothing as no rate limiting is performed.
func (NoopMiddleware) RegisterUser(_ user.Info) error {
	return nil
}
