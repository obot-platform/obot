package server

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/api/authn"
	"github.com/otto8-ai/otto8/pkg/api/authz"
	"github.com/otto8-ai/otto8/pkg/proxy"
	"github.com/otto8-ai/otto8/pkg/storage"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type Server struct {
	storageClient storage.Client
	gptClient     *gptscript.GPTScript
	authenticator *authn.Authenticator
	authorizer    *authz.Authorizer
	proxyServer   *proxy.Proxy

	mux *http.ServeMux
}

func NewServer(storageClient storage.Client, gptClient *gptscript.GPTScript, authn *authn.Authenticator, authz *authz.Authorizer, proxyServer *proxy.Proxy) *Server {
	return &Server{
		storageClient: storageClient,
		gptClient:     gptClient,
		authenticator: authn,
		authorizer:    authz,
		proxyServer:   proxyServer,

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
		if req.Header.Get("X-Otto-Auth-Required") == "true" {
			http.Redirect(rw, req, req.RequestURI, http.StatusFound)
			return
		}

		user, err := s.authenticator.Authenticate(req)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}

		isOAuthPath := strings.HasPrefix(req.URL.Path, "/oauth2/")
		if !s.authorizer.Authorize(req, user) || isOAuthPath {
			// If this is not a request coming from browser or the proxy is not enabled, then return 403.
			if !isOAuthPath && (s.proxyServer == nil || req.Method != http.MethodGet || slices.Contains(user.GetGroups(), authz.AuthenticatedGroup) || !strings.Contains(strings.ToLower(req.UserAgent()), "mozilla")) {
				http.Error(rw, "forbidden", http.StatusForbidden)
				return
			}

			req.Header.Set("X-Otto-Auth-Required", "true")
			s.proxyServer.ServeHTTP(rw, req)
			return
		}

		err = f(api.Context{
			ResponseWriter: rw,
			Request:        req,
			GPTClient:      s.gptClient,
			Storage:        s.storageClient,
			User:           user,
		})

		if errHttp := (*types.ErrHTTP)(nil); errors.As(err, &errHttp) {
			http.Error(rw, errHttp.Message, errHttp.Code)
		} else if errStatus := (*apierrors.StatusError)(nil); errors.As(err, &errStatus) {
			http.Error(rw, errStatus.Error(), int(errStatus.ErrStatus.Code))
		} else if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
		}
	}
}

func GetURLPrefix(req api.Context) string {
	if req.Request.TLS == nil {
		return "http://" + req.Request.Host + "/api"
	}
	return "https://" + req.Request.Host + "/api"
}
