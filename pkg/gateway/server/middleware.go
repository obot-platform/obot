package server

import (
	"fmt"
	"runtime/debug"
	"slices"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/context"
	glog "github.com/obot-platform/obot/pkg/gateway/log"
)

func apply(h api.HandlerFunc, m ...api.Middleware) api.HandlerFunc {
	for _, middleware := range slices.Backward(m) {
		h = middleware(h)
	}
	return h
}

func contentType(contentType string) api.Middleware {
	return func(h api.HandlerFunc) api.HandlerFunc {
		return func(apiContext api.Context) error {
			apiContext.ResponseWriter.Header().Set("Content-Type", contentType)
			return h(apiContext)
		}
	}
}

func logRequest(h api.HandlerFunc) api.HandlerFunc {
	return func(apiContext api.Context) (err error) {
		l := context.GetLogger(apiContext.Context())
		defer func() {
			l.Debug("Handled request", "method", apiContext.Method, "path", apiContext.URL.Path)
			if recErr := recover(); recErr != nil {
				l.Error("Panic", "error", recErr, "stack", string(debug.Stack()))
				err = fmt.Errorf("encountered an unexpected error")
			}
		}()

		l.Debug("Handling request", "method", apiContext.Method, "path", apiContext.URL.Path)
		return h(apiContext)
	}
}

func addRequestID(next api.HandlerFunc) api.HandlerFunc {
	return func(apiContext api.Context) error {
		apiContext.Request = apiContext.WithContext(context.WithNewRequestID(apiContext.Context()))
		return next(apiContext)
	}
}

func addLogger(next api.HandlerFunc) api.HandlerFunc {
	return func(apiContext api.Context) error {
		logger := glog.NewWithID(context.GetRequestID(apiContext.Context()))
		if apiContext.User != nil {
			logger = logger.With("username", apiContext.User.GetName())
		}
		apiContext.Request = apiContext.WithContext(context.WithLogger(
			apiContext.Context(),
			logger,
		))
		return next(apiContext)
	}
}
