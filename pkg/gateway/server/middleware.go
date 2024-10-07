package server

import (
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/gptscript-ai/otto/pkg/api"
	"github.com/gptscript-ai/otto/pkg/gateway/context"
	"github.com/gptscript-ai/otto/pkg/gateway/log"
	"github.com/gptscript-ai/otto/pkg/gateway/types"
	"gorm.io/gorm"
)

type middleware func(http.Handler) http.HandlerFunc

func (s *Server) auth(role types.Role) api.Middleware {
	return func(next api.HandlerFunc) api.HandlerFunc {
		return func(apiContext api.Context) error {
			var (
				authProviderID uint64
				err            error

				logger = context.GetLogger(apiContext.Context())
			)
			if authProviderExtra := apiContext.User.GetExtra()["auth_provider_id"]; len(authProviderExtra) > 0 {
				authProviderID, err = strconv.ParseUint(authProviderExtra[0], 10, 64)
				if err != nil {
					logger.DebugContext(apiContext.Context(), "error parsing auth_provider_id", "error", err)
					writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusUnauthorized, fmt.Errorf("invalid token"))
					return nil
				}
			}

			userID, err := strconv.ParseUint(apiContext.User.GetUID(), 10, 64)
			if err != nil {
				logger.DebugContext(apiContext.Context(), "error parsing user_id", "error", err)
				writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusUnauthorized, fmt.Errorf("invalid token"))
				return nil
			}

			user := new(types.User)
			identity := new(types.Identity)
			if err = s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
				if err := tx.Where("auth_provider_id = ? AND user_id = ?", authProviderID, userID).First(identity).Error; err != nil {
					return err
				}

				return tx.Where("id = ?", userID).First(user).Error
			}); err != nil {
				logger.DebugContext(apiContext.Context(), "error searching for token and user", "error", err)
				writeError(apiContext.Context(), context.GetLogger(apiContext.Context()), apiContext.ResponseWriter, http.StatusUnauthorized, fmt.Errorf("invalid token"))
				return nil
			}

			if !user.Role.HasRole(role) {
				writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusForbidden, fmt.Errorf("forbidden"))
				return nil
			}

			apiContext.Request = apiContext.Request.WithContext(context.WithUser(context.WithIdentity(apiContext.Context(), identity), user))
			return next(apiContext)
		}
	}
}

func (s *Server) authFunc(role types.Role) api.Middleware {
	return func(next api.HandlerFunc) api.HandlerFunc {
		return s.auth(role)(next)
	}
}

func (s *Server) monitor(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var userName string
		logger := context.GetLogger(r.Context())
		if user := context.GetUser(r.Context()); user != nil {
			userName = user.Username
		}

		if err := s.db.WithContext(r.Context()).Create(&types.Monitor{
			CreatedAt: time.Now(),
			Username:  userName,
			Path:      r.URL.Path,
		}).Error; err != nil {
			logger.WarnContext(r.Context(), "error creating monitor", "error", err, "user", userName, "path", r.URL.Path)
		}

		next.ServeHTTP(w, r)
	}
}

func apply(h http.Handler, m ...middleware) http.Handler {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}

func contentType(contentTypes ...string) middleware {
	return func(h http.Handler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			for _, ct := range contentTypes {
				w.Header().Add("Content-Type", ct)
			}
			h.ServeHTTP(w, r)
		}
	}
}

func logRequest(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := context.GetLogger(r.Context())
		defer func() {
			if err := recover(); err != nil {
				l.ErrorContext(r.Context(), "Panic", "error", err, "stack", string(debug.Stack()))
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error": "encountered an unexpected error"}`))
			}
		}()

		l.DebugContext(r.Context(), "Handling request", "method", r.Method, "path", r.URL.Path)
		h.ServeHTTP(w, r)
		l.DebugContext(r.Context(), "Handled request", "method", r.Method, "path", r.URL.Path)
	}
}

func addRequestID(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithNewRequestID(r.Context())))
	}
}

func addLogger(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(
			w,
			r.WithContext(context.WithLogger(
				r.Context(),
				log.NewWithID(context.GetRequestID(r.Context())),
			)),
		)
	}
}

func httpToApiHandlerFunc(handler http.Handler) api.HandlerFunc {
	return func(apiContext api.Context) error {
		handler.ServeHTTP(apiContext.ResponseWriter, apiContext.Request)
		return nil
	}
}
