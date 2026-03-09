package server

import (
	"fmt"
	"runtime/debug"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/context"
	"github.com/obot-platform/obot/pkg/gateway/log"
)

func apply(h api.HandlerFunc, m ...api.Middleware) api.HandlerFunc {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}

func contentType(contentType string) api.Middleware {
	return func(h api.HandlerFunc) api.HandlerFunc {
		return func(apiContext api.Context) error {
			_, span := gatewayTracer.Start(apiContext.Context(), "gateway.middleware.content_type")
			defer span.End()

			apiContext.ResponseWriter.Header().Set("Content-Type", contentType)
			return h(apiContext)
		}
	}
}

func logRequest(h api.HandlerFunc) api.HandlerFunc {
	return func(apiContext api.Context) (err error) {
		_, setupSpan := gatewayTracer.Start(apiContext.Context(), "gateway.middleware.log_request")
		setupSpan.End()

		l := context.GetLogger(apiContext.Context())
		defer func() {
			l.DebugContext(apiContext.Context(), "Handled request", "method", apiContext.Method, "path", apiContext.URL.Path)
			if recErr := recover(); recErr != nil {
				l.ErrorContext(apiContext.Context(), "Panic", "error", err, "stack", string(debug.Stack()))
				err = fmt.Errorf("encountered an unexpected error")
			}
		}()

		l.DebugContext(apiContext.Context(), "Handling request", "method", apiContext.Method, "path", apiContext.URL.Path)
		return h(apiContext)
	}
}

func addRequestID(next api.HandlerFunc) api.HandlerFunc {
	return func(apiContext api.Context) error {
		middlewareCtx, span := gatewayTracer.Start(apiContext.Context(), "gateway.middleware.add_request_id")
		defer span.End()

		apiContext.Request = apiContext.WithContext(context.WithNewRequestID(middlewareCtx))
		return next(apiContext)
	}
}

func addLogger(next api.HandlerFunc) api.HandlerFunc {
	return func(apiContext api.Context) error {
		middlewareCtx, span := gatewayTracer.Start(apiContext.Context(), "gateway.middleware.add_logger")
		defer span.End()

		logger := log.NewWithID(context.GetRequestID(apiContext.Context()))
		if apiContext.User != nil {
			logger = logger.With("username", apiContext.User.GetName())
		}
		apiContext.Request = apiContext.WithContext(context.WithLogger(
			middlewareCtx,
			logger,
		))
		return next(apiContext)
	}
}
