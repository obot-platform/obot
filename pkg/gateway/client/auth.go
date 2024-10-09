package client

import (
	"fmt"
	"net/http"
	"slices"

	"github.com/gptscript-ai/otto/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type UserDecorator struct {
	Next   authenticator.Request
	Client *Client
}

func NewUserDecorator(next authenticator.Request, client *Client) *UserDecorator {
	return &UserDecorator{
		Next:   next,
		Client: client,
	}
}

func (u UserDecorator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	resp, ok, err := u.Next.AuthenticateRequest(req)
	if err != nil {
		return nil, false, err
	} else if !ok {
		return nil, false, nil
	}

	gatewayUser, err := u.Client.EnsureIdentity(req.Context(), &types.Identity{
		Email:            firstValue(resp.User.GetExtra(), "email"),
		AuthProviderID:   uint(firstValueAsInt(resp.User.GetExtra(), "auth_provider_id")),
		ProviderUsername: resp.User.GetName(),
	})
	if err != nil {
		return nil, false, err
	}

	groups := resp.User.GetGroups()
	if gatewayUser.Role == types.RoleAdmin && !slices.Contains(groups, "admin") {
		groups = append(groups, "admin")
	}

	resp.User = &user.DefaultInfo{
		Name:   gatewayUser.Username,
		UID:    fmt.Sprintf("%d", gatewayUser.ID),
		Extra:  resp.User.GetExtra(),
		Groups: groups,
	}
	return resp, true, nil
}
