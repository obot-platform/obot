package server

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/proxy"
	"github.com/obot-platform/obot/pkg/storage"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type Server struct {
	storageClient storage.Client
	gptClient     *gptscript.GPTScript
	authenticator *authn.Authenticator
	authorizer    *authz.Authorizer
	proxyServer   *proxy.Proxy
	baseURL       string

	mux *http.ServeMux
}

func NewServer(storageClient storage.Client, gptClient *gptscript.GPTScript, authn *authn.Authenticator, authz *authz.Authorizer, proxyServer *proxy.Proxy, baseURL string) *Server {
	return &Server{
		storageClient: storageClient,
		gptClient:     gptClient,
		authenticator: authn,
		authorizer:    authz,
		proxyServer:   proxyServer,
		baseURL:       baseURL + "/api",

		mux: http.NewServeMux(),
	}
}

func (s *Server) HandleFunc(pattern string, f api.HandlerFunc) {
	s.mux.HandleFunc(pattern, s.wrap(f))
}

func (s *Server) HTTPHandle(pattern string, f http.Handler) {
	s.HandleFunc(pattern, func(req api.Context) error {
		f.ServeHTTP(req.ResponseWriter, req.Request)
		return nil
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) wrap(f api.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		// If this header is set, then the session was deemed to be invalid and the request has come back around through the proxy.
		// The cookie on the request is still invalid because the new one has not been sent back to the browser.
		// Therefore, respond with a redirect so that the browser will redirect back to the original request with the new cookie.
		if req.Header.Get("X-Obot-Auth-Required") == "true" {
			http.Redirect(rw, req, req.RequestURI, http.StatusFound)
			return
		}

		rw.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, proxy-revalidate, max-age=0")
		rw.Header().Set("Pragma", "no-cache")
		rw.Header().Set("Expires", "0")

		user, err := s.authenticator.Authenticate(req)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}

		isOAuthPath := strings.HasPrefix(req.URL.Path, "/oauth2/")
		if isOAuthPath || strings.HasPrefix(req.URL.Path, "/api/") && !s.authorizer.Authorize(req, user) {
			// If this is not a request coming from browser or the proxy is not enabled, then return 403.
			if !isOAuthPath && (s.proxyServer == nil || req.Method != http.MethodGet || slices.Contains(user.GetGroups(), authz.AuthenticatedGroup) || !strings.Contains(strings.ToLower(req.UserAgent()), "mozilla")) {
				http.Error(rw, "forbidden", http.StatusForbidden)
				return
			}

			req.Header.Set("X-Obot-Auth-Required", "true")
			s.proxyServer.ServeHTTP(rw, req)
			return
		}

		err = f(api.Context{
			ResponseWriter: rw,
			Request:        req,
			GPTClient:      s.gptClient,
			Storage:        s.storageClient,
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
