package api

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/otto/apiclient/types"
	"github.com/gptscript-ai/otto/pkg/gateway/client"
	types2 "github.com/gptscript-ai/otto/pkg/gateway/types"
	"github.com/gptscript-ai/otto/pkg/jwt"
	"github.com/gptscript-ai/otto/pkg/storage"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	user2 "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
)

type Server struct {
	storageClient storage.Client
	gptClient     *gptscript.GPTScript
	gatewayClient *client.Client
	tokenService  *jwt.TokenService
	authenticator authenticator.Request
	authRequired  bool
}

func NewServer(storageClient storage.Client, gptClient *gptscript.GPTScript, gatewayClient *client.Client, tokenService *jwt.TokenService, authn authenticator.Request, authRequired bool) *Server {
	return &Server{
		storageClient: storageClient,
		gptClient:     gptClient,
		gatewayClient: gatewayClient,
		tokenService:  tokenService,
		authenticator: authn,
		authRequired:  authRequired,
	}
}

type (
	HandlerFunc func(Context) error
	Middleware  func(HandlerFunc) HandlerFunc
)

func (s *Server) WrapNoAuth(f HandlerFunc) http.Handler {
	return s.wrap(f, false)
}

func (s *Server) Wrap(f HandlerFunc) http.Handler {
	return s.wrap(f, true)
}

func (s *Server) wrap(f HandlerFunc, authed bool) http.Handler {
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		user, ok := request.UserFrom(req.Context())
		if !ok {
			// In this case, there was no user in the context and the req did not match as an unauthenticated path.
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
			} else if authed && s.authenticator != nil {
				resp, ok, err := s.authenticator.AuthenticateRequest(req)
				if s.authRequired {
					if err != nil {
						http.Error(rw, err.Error(), http.StatusUnauthorized)
						return
					} else if !ok {
						http.Error(rw, "Unauthorized", http.StatusUnauthorized)
						return
					}
				}

				if resp != nil {
					gatewayUser, err := s.gatewayClient.EnsureIdentity(req.Context(), &types2.Identity{
						Email:            firstValue(resp.User.GetExtra(), "email"),
						AuthProviderID:   uint(firstValueAsInt(resp.User.GetExtra(), "auth_provider_id")),
						ProviderUsername: resp.User.GetName(),
					})
					if err != nil {
						http.Error(rw, err.Error(), http.StatusInternalServerError)
						return
					}

					groups := resp.User.GetGroups()
					if gatewayUser.Role == types2.RoleAdmin && !slices.Contains(groups, "admin") {
						groups = append(groups, "admin")
					}

					user = &user2.DefaultInfo{
						Name:   gatewayUser.Username,
						UID:    fmt.Sprintf("%d", gatewayUser.ID),
						Extra:  resp.User.GetExtra(),
						Groups: groups,
					}
				}
			} else if authed && s.authRequired {
				http.Error(rw, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		if user == nil {
			user = &user2.DefaultInfo{}
		}

		err := f(Context{
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

func firstValue(m map[string][]string, key string) string {
	values := m[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstValueAsInt(m map[string][]string, key string) int {
	value := firstValue(m, key)
	v, _ := strconv.Atoi(value)
	return v
}
