package server

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (s *Server) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	bearer := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if bearer == "" {
		return nil, false, nil
	}

	id, token, _ := strings.Cut(bearer, ":")
	u := new(types.User)
	var namespace, name string
	if err := s.db.WithContext(req.Context()).Transaction(func(tx *gorm.DB) error {
		tkn := new(types.AuthToken)
		if err := tx.Where("id = ? AND hashed_token = ?", id, hashToken(token)).First(tkn).Error; err != nil {
			return err
		}

		namespace = tkn.AuthProviderNamespace
		name = tkn.AuthProviderName
		return tx.Where("id = ?", tkn.UserID).First(u).Error
	}); err != nil {
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
