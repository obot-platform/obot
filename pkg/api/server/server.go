package server

import (
	"bufio"
	"cmp"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/server/audit"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
	"github.com/obot-platform/obot/pkg/api/server/requestinfo"
	"github.com/obot-platform/obot/pkg/auth"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/proxy"
	"github.com/obot-platform/obot/pkg/scim"
	"github.com/obot-platform/obot/pkg/storage"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
)

const (
	// accountInactiveLoginPath is the login page that a refused page load is sent to, with accountInactiveCookie. Its
	// parameter tells a refused load of the login page itself apart, which is not sent there again.
	accountInactiveLoginPath = "/?inactive=true"
	// accountInactiveCookie tells the login page that its visitor's account is not active. The UI's root layout reads
	// and clears it (ACCOUNT_INACTIVE_COOKIE in ui/user/src/routes/+layout.ts).
	accountInactiveCookie = "obot_account_inactive"
)

type Server struct {
	storageClient           storage.Client
	gatewayClient           *gclient.Client
	localK8sClient          kclient.Client
	obotNamespace           string
	authenticator           *authn.Authenticator
	authorizer              *authz.Authorizer
	proxyManager            *proxy.Manager
	auditLogger             audit.Logger
	rateLimiter             *ratelimiter.RateLimiter
	baseURL                 string
	mcpOAuthScope           string
	registryNoAuth          bool
	providerEntitlementGate *license.ProviderEntitlementGate

	mux         *http.ServeMux
	otelHandler http.Handler
	// scimHandler serves the SCIM endpoint, which HandleSCIM sets.
	scimHandler http.Handler
}

type headersResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

type responseWriter struct {
	http.ResponseWriter
	auditEntry  audit.LogEntry
	auditLogger audit.Logger
}

func NewServer(storageClient storage.Client, gatewayClient *gclient.Client, localK8sClient kclient.Client, obotNamespace string, authn *authn.Authenticator, authz *authz.Authorizer, proxyManager *proxy.Manager, auditLogger audit.Logger, rateLimiter *ratelimiter.RateLimiter, baseURL string, oauthScopesSupported []string, registryNoAuth bool, licenseProvider *license.Provider) *Server {
	var scope string
	if len(oauthScopesSupported) > 0 {
		scope = fmt.Sprintf(", scope=\"%s\"", strings.Join(oauthScopesSupported, " "))
	}
	s := &Server{
		storageClient:           storageClient,
		gatewayClient:           gatewayClient,
		localK8sClient:          localK8sClient,
		obotNamespace:           obotNamespace,
		authenticator:           authn,
		authorizer:              authz,
		proxyManager:            proxyManager,
		baseURL:                 baseURL + "/api",
		mcpOAuthScope:           scope,
		auditLogger:             auditLogger,
		rateLimiter:             rateLimiter,
		registryNoAuth:          registryNoAuth,
		mux:                     http.NewServeMux(),
		providerEntitlementGate: license.NewProviderEntitlementGate(licenseProvider, storageClient),
	}
	s.otelHandler = traced(s.mux)
	return s
}

// authenticationError finds an error of type T in an error from the authenticator chain. The chain's unions
// aggregate their members' errors without unwrapping them, so errors.As alone cannot see through them.
func authenticationError[T error](err error) (T, bool) {
	if target, ok := errors.AsType[T](err); ok {
		return target, true
	}
	if aggregate, ok := errors.AsType[utilerrors.Aggregate](err); ok {
		for _, err := range aggregate.Errors() {
			if target, ok := authenticationError[T](err); ok {
				return target, true
			}
		}
	}
	var zero T
	return zero, false
}

func (s *Server) HandleFunc(pattern string, f api.HandlerFunc) {
	s.mux.Handle(pattern, s.Wrap(f))
}

// HandleSCIM serves the SCIM endpoint with f. SCIM requests bypass the mux, which would answer the base URL without a
// trailing slash, and any path it would clean, such as one with a doubled slash, with a redirect that no SCIM client
// expects, rather than with a SCIM response.
func (s *Server) HandleSCIM(f api.HandlerFunc) {
	wrapped := s.Wrap(f)
	s.scimHandler = traced(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		// The password change check and tracing read the pattern that the mux would have matched.
		req.Pattern = scim.PathPrefix
		wrapped(rw, req)
	}))
}

func (s *Server) HTTPHandle(pattern string, f http.Handler) {
	s.HandleFunc(pattern, func(req api.Context) error {
		f.ServeHTTP(req.ResponseWriter, req.Request)
		return nil
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.scimHandler != nil {
		if scim.IsSCIMPath(r.URL.Path) {
			s.scimHandler.ServeHTTP(w, r)
			return
		}
		// A path that is a SCIM path only once cleaned, such as one that a server URL with a trailing slash leaves
		// beginning with a doubled slash, would get the mux's redirect to the cleaned path. It is served as that path.
		if cleaned := path.Clean(r.URL.Path); scim.IsSCIMPath(cleaned) {
			r = r.Clone(r.Context())
			r.URL.Path = cleaned
			r.URL.RawPath = ""
			s.scimHandler.ServeHTTP(w, r)
			return
		}
	}
	s.otelHandler.ServeHTTP(w, r)
}

// traced traces the requests that h serves, other than health checks and static assets.
func traced(h http.Handler) http.Handler {
	return otelhttp.NewHandler(
		h,
		"obot/http",
		otelhttp.WithFilter(func(r *http.Request) bool {
			return r.URL.Path != "/api/healthz" && !isStaticAssetPath(r.URL.Path)
		}),
		otelhttp.WithSpanNameFormatter(func(operation string, r *http.Request) string {
			if r.Pattern == "" {
				return operation
			}
			return r.Pattern
		}),
	)
}

func (s *Server) Wrap(f api.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		// Ensure security headers and a sane default Content-Type.
		// This wrapper is intentionally applied early so it covers authn/authz
		// errors, registry endpoints, UI, static, and proxy responses.
		rw = &headersResponseWriter{ResponseWriter: rw}

		// SCIM clients understand only SCIM responses, so every failure on a SCIM route is written as one.
		isSCIM := scim.IsSCIMPath(req.URL.Path)

		user, err := s.authenticator.Authenticate(req)
		if err != nil {
			if isSCIM {
				writeSCIMAuthenticationError(rw, err)
				return
			}

			if errors.Is(err, proxy.ErrInvalidSession) {
				// The session is invalid, so tell the browser to delete the cookie so that it won't try it again.
				http.SetCookie(rw, &http.Cookie{
					Name:   proxy.ObotAccessTokenCookie,
					Value:  "",
					Path:   "/",
					MaxAge: -1,
				})
				auth.ClearAuthProviderVerifyCookie(rw)
				// Refresh the page so that the cookie deletes.
				http.Redirect(rw, req, req.URL.String(), http.StatusFound)
				return
			}

			// Check if this is a FetchUserGroupsError which indicates an auth provider configuration issue
			if fetchGroupsErr, ok := authenticationError[*gclient.FetchUserGroupsError](err); ok {
				http.Error(rw, fmt.Sprintf("Authentication provider configuration error: %s. Please contact an administrator to fix the auth provider configuration.", fetchGroupsErr.Message), http.StatusInternalServerError)
			} else if denied, ok := authenticationError[*gclient.UserAccessDeniedError](err); ok {
				// End the browser session, so that the login page is reachable and can say why.
				http.SetCookie(rw, &http.Cookie{
					Name:   proxy.ObotAccessTokenCookie,
					Value:  "",
					Path:   "/",
					MaxAge: -1,
				})
				// A browser would show the refusal of a page as a bare text page, and the UI that explains it would
				// never load, so the browser is sent to the login page instead, which says why.
				if isPageLoad(req) && req.URL.String() != accountInactiveLoginPath {
					http.SetCookie(rw, &http.Cookie{
						Name:     accountInactiveCookie,
						Value:    "true",
						Path:     "/",
						MaxAge:   60,
						SameSite: http.SameSiteLaxMode,
					})
					http.Redirect(rw, req, accountInactiveLoginPath, http.StatusFound)
					return
				}
				httpErr := denied.HTTPError()
				http.Error(rw, httpErr.Message, httpErr.Code)
			} else if lookupErr, ok := authenticationError[*gclient.UserAccessLookupError](err); ok {
				slog.Error("Denied request because the user's status could not be checked", "userID", lookupErr.UserID, "error", lookupErr.Err)
				http.Error(rw, "Unable to verify account status. Please try again.", http.StatusServiceUnavailable)
			} else {
				http.Error(rw, err.Error(), http.StatusUnauthorized)
			}

			return
		}
		// The admission check let the principal through, so work done for the request can trust its recorded status.
		req = req.WithContext(principal.WithAdmittedPrincipal(req.Context(), user))

		// Skip rate limiting for static assets (JS chunks, CSS, images) to avoid
		// hitting limits during page load when many assets are fetched in parallel.
		if !isStaticAssetPath(req.URL.Path) {
			if err := s.rateLimiter.ApplyLimit(user, rw, req); err != nil {
				if errors.Is(err, ratelimiter.ErrRateLimitExceeded) {
					// The user has exceeded their rate limit.
					if isSCIM {
						scim.WriteError(rw, http.StatusTooManyRequests, err.Error())
					} else {
						http.Error(rw, err.Error(), http.StatusTooManyRequests)
					}
					return
				}

				// There was an error applying the rate limit.
				// Log it and move on so that a failure to apply rate limits doesn't take down the entire API.
				slog.Warn("Failed to apply rate limits", "error", err)
			}
		}

		authenticated := !slices.Contains(user.GetGroups(), authz.UnauthenticatedGroup)
		isAPI := strings.HasPrefix(req.URL.Path, "/api/") && req.URL.Path != "/api/healthz"
		if isAPI || isSCIM {
			// Setup a new response writer for audit logging. Its entries never include the query string or the
			// body, which on SCIM routes can hold identity provider data.
			rw = &responseWriter{
				ResponseWriter: rw,
				auditEntry: audit.LogEntry{
					Time:      time.Now(),
					UserID:    user.GetUID(),
					Method:    req.Method,
					Path:      req.URL.Path,
					UserAgent: req.UserAgent(),
					SourceIP:  requestinfo.GetSourceIP(req),
					Host:      req.Host,
				},
				auditLogger: s.auditLogger,
			}
		}
		if isAPI {
			// SCIM routes are outside /api/, so a SCIM connection's requests are never recorded as user activity.
			if authenticated {
				// Best effort
				if err := s.gatewayClient.AddActivityForToday(req.Context(), user.GetUID()); err != nil {
					slog.Warn("Failed to add activity tracking for user", "user", user.GetName(), "error", err)
				}
			}
		}

		if user.GetExtra()["set-cookies"] != nil {
			for _, setCookie := range user.GetExtra()["set-cookies"] {
				rw.Header().Add("Set-Cookie", setCookie)
			}
		}

		// Enforced after audit logging is installed and refreshed provider cookies are replayed, so
		// rejected probes stay auditable and a cookie refresh is not lost to a blocked operation.
		if cmp.Or(user.GetExtra()["password_change_required"]...) == "true" && !passwordChangeRequestAllowed(req) {
			if req.Pattern != "/" {
				http.Error(rw, "password change required", http.StatusForbidden)
			} else {
				rd := req.URL.RequestURI()
				http.Redirect(rw, req, "/change-password?rd="+url.QueryEscape(rd), http.StatusSeeOther)
			}
			return
		}

		var shouldLogError bool
		err = s.providerEntitlementGate.Check(req)
		if err == nil {
			if !s.authorizer.Authorize(req, user) {
				if _, err := req.Cookie(auth.ObotAccessTokenCookie); err == nil && req.URL.Path == "/api/me" {
					// Tell the browser to delete the obot_access_token cookie.
					// If the user tried to access this path and was unauthorized, then something is wrong with their token.
					http.SetCookie(rw, &http.Cookie{
						Name:   auth.ObotAccessTokenCookie,
						Value:  "",
						Path:   "/",
						MaxAge: -1,
					})
				}

				// Only set WWW-Authenticate if not in no-auth mode
				if strings.HasPrefix(req.URL.Path, "/v0.1") && !s.registryNoAuth {
					rw.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="MCP Registry", resource_metadata="%s/.well-known/oauth-protected-resource/v0.1/servers"`, strings.TrimSuffix(s.baseURL, "/api")))
				}

				switch {
				case isSCIM && authenticated:
					scim.WriteError(rw, http.StatusForbidden, "this credential cannot access this SCIM endpoint")
				case isSCIM:
					scim.WriteUnauthorized(rw)
				case authenticated:
					http.Error(rw, "forbidden", http.StatusForbidden)
				default:
					http.Error(rw, "unauthorized", http.StatusUnauthorized)
				}

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
				Storage:        s.storageClient,
				GatewayClient:  s.gatewayClient,
				User:           user,
				APIBaseURL:     s.baseURL,
				LocalK8sClient: s.localK8sClient,
				ObotNamespace:  s.obotNamespace,
			})
		}
		writeError := http.Error
		if isSCIM {
			writeError = func(w http.ResponseWriter, message string, code int) {
				scim.WriteError(w, code, message)
			}
		}
		if errHTTP := (*types.ErrHTTP)(nil); errors.As(err, &errHTTP) {
			writeError(rw, errHTTP.Message, errHTTP.Code)
			shouldLogError = errHTTP.Code == http.StatusInternalServerError
		} else if errStatus := (*apierrors.StatusError)(nil); errors.As(err, &errStatus) {
			writeError(rw, errStatus.Error(), int(errStatus.ErrStatus.Code))
			shouldLogError = errStatus.ErrStatus.Code == http.StatusInternalServerError
		} else if err != nil {
			writeError(rw, err.Error(), http.StatusInternalServerError)
			shouldLogError = true
		}

		if shouldLogError {
			slog.Error("Error handling request", "path", req.URL.Path, "error", err)
		}
	}
}

// writeSCIMAuthenticationError answers a SCIM request whose authentication failed. Without any SCIM connection, the
// endpoint is unavailable. Any other failure is Obot's, and is never reported as a bad credential.
func writeSCIMAuthenticationError(rw http.ResponseWriter, err error) {
	if _, ok := authenticationError[*scim.UnavailableError](err); ok {
		scim.WriteUnavailable(rw, "SCIM is not enabled")
		return
	}

	slog.Error("Failed to authenticate SCIM request", "error", err)
	scim.WriteError(rw, http.StatusInternalServerError, "internal error")
}

func passwordChangeRequestAllowed(req *http.Request) bool {
	if isStaticAssetPath(req.URL.Path) || req.URL.Path == "/change-password" || strings.HasPrefix(req.URL.Path, "/change-password/") || req.URL.Path == "/activate" || strings.HasPrefix(req.URL.Path, "/activate/") || req.URL.Path == "/oauth2/sign_out" {
		return true
	}
	return (req.Method == http.MethodGet && slices.Contains([]string{
		"/api/me",
		"/api/version",
		"/api/license",
		"/api/app-preferences",
	}, req.URL.Path)) ||
		(req.Method == http.MethodPost && slices.Contains([]string{
			"/api/local-auth/activate",
			"/api/local-auth/change-password",
		}, req.URL.Path))
}

func (w *headersResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *headersResponseWriter) ensureHeaders(status int) {
	// Always set nosniff; harmless for non-browser clients.
	w.Header().Set("X-Content-Type-Options", "nosniff")

	// If a handler is going to send a body, ensure Content-Type is present.
	// Avoid setting it for statuses that must not include a body.
	if (status >= 100 && status < 200) || status == http.StatusNoContent || status == http.StatusResetContent || status == http.StatusNotModified {
		return
	}
	if w.Header().Get("Content-Type") == "" {
		// Use the same default net/http uses for plain text errors.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	}
}

func (w *headersResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.ensureHeaders(status)
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *headersResponseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *headersResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(w, r)
}

func (w *headersResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *headersResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
	}
	return h.Hijack()
}

func (w *headersResponseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// isPageLoad reports whether a browser is loading a page, rather than calling the API or fetching an asset.
func isPageLoad(req *http.Request) bool {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		return false
	}
	if strings.HasPrefix(req.URL.Path, "/api/") || isStaticAssetPath(req.URL.Path) {
		return false
	}
	return strings.Contains(req.Header.Get("Accept"), "text/html")
}

// isStaticAssetPath returns true if the path is a static asset that should be
// exempt from rate limiting. This includes SvelteKit chunks, CSS, and UI images.
func isStaticAssetPath(path string) bool {
	return strings.HasPrefix(path, "/_app/") || strings.HasPrefix(path, "/user/images/") || slices.Contains([]string{
		"/favicon.ico",
		"/favicon-16x16.png",
		"/favicon-32x32.png",
		"/apple-touch-icon.png",
		"/android-chrome-192x192.png",
		"/android-chrome-512x512.png",
	}, path)
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.auditEntry.ResponseCode = code
	rw.ResponseWriter.WriteHeader(code)

	if err := rw.auditLogger.LogEntry(rw.auditEntry); err != nil {
		slog.Error("Failed to log audit entry", "error", err)
	}
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap exposes the writer underneath to http.ResponseController. Without it a
// websocket handler under /api/ cannot hijack the connection, because this
// wrapper hides the Hijacker the server provides.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
