package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/ratelimit"
	"github.com/obot-platform/obot/pkg/api/server/audit"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/proxy"
	"github.com/obot-platform/obot/pkg/storage"
	"go.opentelemetry.io/otel"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var (
	log    = logger.Package()
	tracer = otel.Tracer("obot/api")
)

type Server struct {
	storageClient storage.Client
	gatewayClient *gclient.Client
	gptClient     *gptscript.GPTScript
	rateLimiter   ratelimit.Middleware
	authenticator *authn.Authenticator
	authorizer    *authz.Authorizer
	proxyManager  *proxy.Manager
	auditLogger   audit.Logger
	baseURL       string

	mux *http.ServeMux
}

func NewServer(
	storageClient storage.Client,
	gatewayClient *gclient.Client,
	gptClient *gptscript.GPTScript,
	rateLimiter ratelimit.Middleware,
	authn *authn.Authenticator,
	authz *authz.Authorizer,
	proxyManager *proxy.Manager,
	auditLogger audit.Logger,
	baseURL string,
) *Server {
	return &Server{
		storageClient: storageClient,
		gatewayClient: gatewayClient,
		gptClient:     gptClient,
		rateLimiter:   rateLimiter,
		authenticator: authn,
		authorizer:    authz,
		proxyManager:  proxyManager,
		baseURL:       baseURL + "/api",
		auditLogger:   auditLogger,
		mux:           http.NewServeMux(),
	}
}

func (s *Server) HandleFunc(pattern string, f api.HandlerFunc) {
	s.mux.Handle(pattern, http.HandlerFunc(s.wrap(f)))
}

func (s *Server) HTTPHandle(pattern string, f http.Handler) {
	s.HandleFunc(pattern, func(req api.Context) error {
		f.ServeHTTP(req.ResponseWriter, req.Request)
		return nil
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "server")
	defer span.End()
	s.rateLimiter.Limit(s.mux).ServeHTTP(w, r.WithContext(ctx))
}

func (s *Server) wrap(f api.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		ctx, span := tracer.Start(req.Context(), req.Pattern)
		defer span.End()
		req = req.WithContext(ctx)

		user, err := s.authenticator.Authenticate(req)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}

		// Register user with the rate limiter to set the appropriate rate limit.
		if err := s.rateLimiter.RegisterUser(user); err != nil {
			log.Errorf("Failed to register user for rate limiting: %v", err)
		}

		if strings.HasPrefix(req.URL.Path, "/api/") {
			// Setup a new response writer for audit logging.
			rw = &responseWriter{
				ResponseWriter: rw,
				auditEntry: audit.LogEntry{
					Time:      time.Now(),
					UserID:    user.GetUID(),
					Method:    req.Method,
					Path:      req.URL.Path,
					UserAgent: req.UserAgent(),
					SourceIP:  getSourceIP(req),
					Host:      req.Host,
				},
				auditLogger: s.auditLogger,
			}

			if user.GetUID() != "" && user.GetUID() != "anonymous" {
				// Best effort
				if err := s.gatewayClient.AddActivityForToday(req.Context(), user.GetUID()); err != nil {
					log.Warnf("Failed to add activity tracking for user %s: %v", user.GetName(), err)
				}
			}
		}

		if user.GetExtra()["set-cookies"] != nil {
			for _, setCookie := range user.GetExtra()["set-cookies"] {
				rw.Header().Add("Set-Cookie", setCookie)
			}
		}

		if !s.authorizer.Authorize(req, user) {
			if _, err := req.Cookie("obot_access_token"); err == nil && req.URL.Path == "/api/me" {
				// Tell the browser to delete the obot_access_token cookie.
				// If the user tried to access this path and was unauthorized, then something is wrong with their token.
				http.SetCookie(rw, &http.Cookie{
					Name:   "obot_access_token",
					Value:  "",
					Path:   "/",
					MaxAge: -1,
				})
			}

			http.Error(rw, "forbidden", http.StatusForbidden)
			return
		}

		if strings.HasPrefix(req.URL.Path, "/api/") {
			rw.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
			rw.Header().Set("Pragma", "no-cache")
			rw.Header().Set("Expires", "0")
		}

		err = f(api.Context{
			ResponseWriter: rw,
			Request:        req,
			GPTClient:      s.gptClient,
			Storage:        s.storageClient,
			GatewayClient:  s.gatewayClient,
			User:           user,
			APIBaseURL:     s.baseURL,
		})
		if errHTTP := (*types.ErrHTTP)(nil); errors.As(err, &errHTTP) {
			http.Error(rw, errHTTP.Message, errHTTP.Code)
		} else if errStatus := (*apierrors.StatusError)(nil); errors.As(err, &errStatus) {
			http.Error(rw, errStatus.Error(), int(errStatus.ErrStatus.Code))
		} else if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
	}
}

type responseWriter struct {
	http.ResponseWriter
	auditEntry  audit.LogEntry
	auditLogger audit.Logger
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.auditEntry.ResponseCode = code
	rw.ResponseWriter.WriteHeader(code)

	if err := rw.auditLogger.LogEntry(rw.auditEntry); err != nil {
		log.Errorf("Failed to log audit entry: %v", err)
	}
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// getSourceIP extracts the real client IP address from the request.
// It checks X-Forwarded-For and X-Real-IP headers before falling back to RemoteAddr.
func getSourceIP(req *http.Request) string {
	// Check X-Forwarded-For header first
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For can contain multiple IPs (client, proxy1, proxy2, ...)
		// The leftmost one is the original client IP
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header next
	if xrip := req.Header.Get("X-Real-IP"); xrip != "" {
		return xrip
	}

	// Fall back to RemoteAddr
	return req.RemoteAddr
}
