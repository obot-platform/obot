package mcpserver

import (
	"context"
	"net/http"
)

type internalContextKey struct{}

func checkInternalRequestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for internal query parameter to identify internal requests
		if r.URL.Query().Get("internal") == "true" {
			// Mark the context as internal
			r = r.WithContext(internalContext(r.Context()))
		}
		next.ServeHTTP(w, r)
	})
}

func internalContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, internalContextKey{}, struct{}{})
}

func isInternalContext(ctx context.Context) bool {
	_, ok := ctx.Value(internalContextKey{}).(struct{})
	return ok
}
