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
			apiContext.ResponseWriter.Header().Set("Content-Type", contentType)
			return h(apiContext)
		}
	}
}

func logRequest(h api.HandlerFunc) api.HandlerFunc {
	return func(apiContext api.Context) (err error) {
		l := context.GetLogger(apiContext.Context())
		defer func() {
			l.Debugf("Handled request: method=%s path=%s", apiContext.Method, apiContext.URL.Path)
			if recErr := recover(); recErr != nil {
				l.Errorf("Panic: error=%v stack=%s", recErr, string(debug.Stack()))
				err = fmt.Errorf("encountered an unexpected error")
			}
		}()

		l.Debugf("Handling request: method=%s path=%s", apiContext.Method, apiContext.URL.Path)
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
		logger := log.NewWithID(context.GetRequestID(apiContext.Context()))
		if apiContext.User != nil {
			logger = logger.Fields("username", apiContext.User.GetName())
		}
		apiContext.Request = apiContext.WithContext(context.WithLogger(
			apiContext.Context(),
			logger,
		))
		return next(apiContext)
	}
}
