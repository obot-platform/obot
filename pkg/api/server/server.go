package server

import (
	"errors"
	"net/http"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	"github.com/otto8-ai/otto8/pkg/api/authn"
	"github.com/otto8-ai/otto8/pkg/api/authz"
	"github.com/otto8-ai/otto8/pkg/storage"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type Server struct {
	storageClient storage.Client
	gptClient     *gptscript.GPTScript
	authenticator *authn.Authenticator
	authorizer    *authz.Authorizer

	mux *http.ServeMux
}

func NewServer(storageClient storage.Client, gptClient *gptscript.GPTScript, authn *authn.Authenticator, authz *authz.Authorizer) *Server {
	return &Server{
		storageClient: storageClient,
		gptClient:     gptClient,
		authenticator: authn,
		authorizer:    authz,

		mux: http.NewServeMux(),
	}
}

func (s *Server) HandleFunc(pattern string, f api.HandlerFunc) {
	s.mux.HandleFunc(pattern, s.wrap(f))
}

func (s *Server) HTTPHandle(pattern string, f http.Handler) {
	s.mux.Handle(pattern, f)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) wrap(f api.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, req *http.Request) {
		user, err := s.authenticator.Authenticate(req)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}

		if !s.authorizer.Authorize(req, user) {
			http.Error(rw, "forbidden", http.StatusForbidden)
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
		return "http://" + req.Request.Host
	}
	return "https://" + req.Request.Host
}
