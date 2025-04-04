package server

import (
	"net/http"
	"strconv"
	"strings"

	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (s *Server) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	bearer := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if bearer == "" {
		return nil, false, nil
	}

	u, namespace, name, err := s.client.UserFromToken(req.Context(), bearer)
	if err != nil {
		return nil, false, err
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name: u.Username,
			UID:  strconv.FormatUint(uint64(u.ID), 10),
			Extra: map[string][]string{
				"email":                   {u.Email},
				"auth_provider_namespace": {namespace},
				"auth_provider_name":      {name},
			},
		},
	}, true, nil
}
