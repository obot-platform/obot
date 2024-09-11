package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/otto/pkg/jwt"
	"github.com/gptscript-ai/otto/pkg/storage"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	user2 "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
)

type Server struct {
	client       storage.Client
	gptClient    *gptscript.GPTScript
	tokenService *jwt.TokenService
}

func NewServer(client storage.Client, gptClient *gptscript.GPTScript, tokenService *jwt.TokenService) *Server {
	return &Server{
		client:       client,
		gptClient:    gptClient,
		tokenService: tokenService,
	}
}

type HandlerFunc func(Context) error

func (s *Server) Wrap(f HandlerFunc) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		user, ok := request.UserFrom(req.Context())
		if !ok {
			token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
			tokenContext, err := s.tokenService.DecodeToken(token)
			if err == nil {
				user = &user2.DefaultInfo{
					Name: tokenContext.Scope,
					Extra: map[string][]string{
						"otto:runID":    {tokenContext.RunID},
						"otto:threadID": {tokenContext.ThreadID},
						"otto:agentID":  {tokenContext.AgentID},
					},
				}
			} else {
				user = &user2.DefaultInfo{}
			}
		}

		err := f(Context{
			ResponseWriter: rw,
			Request:        req,
			GPTClient:      s.gptClient,
			Storage:        s.client,
			User:           user,
		})

		if errHttp := (*ErrHTTP)(nil); errors.As(err, &errHttp) {
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
