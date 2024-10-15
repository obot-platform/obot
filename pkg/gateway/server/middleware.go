package server

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/gateway/context"
	"github.com/otto8-ai/otto8/pkg/gateway/log"
	"github.com/otto8-ai/otto8/pkg/gateway/types"
)

func (s *Server) monitor(next api.HandlerFunc) api.HandlerFunc {
	return func(apiContext api.Context) error {
		logger := context.GetLogger(apiContext.Context())
		if err := s.db.WithContext(apiContext.Context()).Create(&types.Monitor{
			CreatedAt: time.Now(),
			Username:  apiContext.User.GetName(),
			Path:      apiContext.URL.Path,
		}).Error; err != nil {
			logger.WarnContext(apiContext.Context(), "error creating monitor", "error", err, "user", apiContext.User.GetName(), "path", apiContext.URL.Path)
		}

		return next(apiContext)
	}
}

func apply(h api.HandlerFunc, m ...api.Middleware) api.HandlerFunc {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}

func contentType(contentTypes ...string) api.Middleware {
	return func(h api.HandlerFunc) api.HandlerFunc {
		return func(apiContext api.Context) error {
			for _, ct := range contentTypes {
				apiContext.ResponseWriter.Header().Add("Content-Type", ct)
			}
			return h(apiContext)
		}
	}
}

func logRequest(h api.HandlerFunc) api.HandlerFunc {
	return func(apiContext api.Context) (err error) {
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
		apiContext.Request = apiContext.Request.WithContext(context.WithNewRequestID(apiContext.Request.Context()))
		return next(apiContext)
	}
}

func addLogger(next api.HandlerFunc) api.HandlerFunc {
	return func(apiContext api.Context) error {
		logger := log.NewWithID(context.GetRequestID(apiContext.Request.Context()))
		if apiContext.User != nil {
			logger = logger.With("username", apiContext.User.GetName())
		}
		apiContext.Request = apiContext.Request.WithContext(context.WithLogger(
			apiContext.Request.Context(),
			logger,
		))
		return next(apiContext)
	}
}

func httpToApiHandlerFunc(handler http.Handler) api.HandlerFunc {
	return func(apiContext api.Context) error {
		handler.ServeHTTP(apiContext.ResponseWriter, apiContext.Request)
		return nil
	}
}
