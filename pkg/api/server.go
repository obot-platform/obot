package api

import (
	"errors"
	"net/http"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/otto/apiclient/types"
	"github.com/gptscript-ai/otto/pkg/gateway/client"
	"github.com/gptscript-ai/otto/pkg/jwt"
	"github.com/gptscript-ai/otto/pkg/storage"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type Server struct {
	storageClient storage.Client
	gptClient     *gptscript.GPTScript
	gatewayClient *client.Client
	tokenService  *jwt.TokenService
	authenticator authenticator.Request
}

func NewServer(storageClient storage.Client, gptClient *gptscript.GPTScript, gatewayClient *client.Client, tokenService *jwt.TokenService, authn authenticator.Request) *Server {
	return &Server{
		storageClient: storageClient,
		gptClient:     gptClient,
		gatewayClient: gatewayClient,
		tokenService:  tokenService,
		authenticator: authn,
	}
}

type (
	HandlerFunc func(Context) error
	Middleware  func(HandlerFunc) HandlerFunc
)

func (s *Server) getUser(req *http.Request) (user.Info, error) {
	resp, ok, err := s.authenticator.AuthenticateRequest(req)
	if err != nil {
		return nil, err
	}
	if !ok {
		panic("authentication should always succeed")
	}
	return resp.User, nil
}

func (s *Server) Wrap(f HandlerFunc) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		user, err := s.getUser(req)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusUnauthorized)
			return
		}
		err = f(Context{
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
	})
}

func GetURLPrefix(req Context) string {
	if req.Request.TLS == nil {
		return "http://" + req.Request.Host
	}
	return "https://" + req.Request.Host
}
