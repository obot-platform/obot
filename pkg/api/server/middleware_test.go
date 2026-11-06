package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
	"github.com/stretchr/testify/require"
)

func TestWithMiddlewareScopesRegistrations(t *testing.T) {
	limiter, err := ratelimiter.New(ratelimiter.Options{UnauthenticatedRateLimit: 1000})
	require.NoError(t, err)
	s := &Server{
		mux:           http.NewServeMux(),
		authenticator: authn.NewAuthenticator(authn.Anonymous{}),
		authorizer:    authz.NewAuthorizer(nil, nil, nil, false, nil, nil, &registrySettings{enabled: true}),
		rateLimiter:   limiter,
	}
	mark := func(name string) func(api.HandlerFunc) api.HandlerFunc {
		return func(next api.HandlerFunc) api.HandlerFunc {
			return func(req api.Context) error {
				req.ResponseWriter.Header().Add("X-Middleware", name)
				return next(req)
			}
		}
	}
	parent := s.WithMiddleware(mark("parent"))
	child := parent.WithMiddleware(mark("child"))
	sibling := parent.WithMiddleware(mark("sibling"))
	combined := parent.WithMiddleware(mark("first"), mark("second"))
	handler := func(req api.Context) error { return req.Write(req.PathValue("name")) }
	s.HandleFunc("GET /v0.1/plain/{name}", handler)
	parent.HandleFunc("GET /v0.1/parent/{name}", handler)
	child.HandleFunc("GET /v0.1/child/{name}", handler)
	sibling.HandleFunc("GET /v0.1/sibling/{name}", handler)
	combined.HandleFunc("GET /v0.1/combined/{name}", handler)
	for _, tc := range []struct {
		path string
		want string
	}{
		{
			path: "plain",
		},
		{
			path: "parent",
			want: "parent",
		},
		{
			path: "child",
			want: "parent,child",
		},
		{
			path: "sibling",
			want: "parent,sibling",
		},
		{
			path: "combined",
			want: "parent,first,second",
		},
	} {
		t.Run(tc.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			s.mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/v0.1/"+tc.path+"/example", nil))
			require.Equal(t, http.StatusOK, response.Code)
			require.Equal(t, tc.want, strings.Join(response.Header().Values("X-Middleware"), ","))
			require.Contains(t, response.Body.String(), "example")
		})
	}
}
