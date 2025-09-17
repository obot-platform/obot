package client

import (
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type UserDecorator struct {
	next   authenticator.Request
	client *Client
}

func NewUserDecorator(next authenticator.Request, client *Client) *UserDecorator {
	return &UserDecorator{
		next:   next,
		client: client,
	}
}

func (u UserDecorator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	resp, ok, err := u.next.AuthenticateRequest(req)
	if err != nil {
		return nil, false, err
	} else if !ok {
		return nil, false, nil
	}

	identity := &types.Identity{
		Email:                 auth.FirstExtraValue(resp.User.GetExtra(), "email"),
		AuthProviderName:      auth.FirstExtraValue(resp.User.GetExtra(), "auth_provider_name"),
		AuthProviderNamespace: auth.FirstExtraValue(resp.User.GetExtra(), "auth_provider_namespace"),
		ProviderUsername:      resp.User.GetName(),
		ProviderUserID:        resp.User.GetUID(),
	}
	gatewayUser, err := u.client.EnsureIdentity(req.Context(), identity, req.Header.Get("X-Obot-User-Timezone"))
	if err != nil {
		return nil, false, err
	}

	extra := resp.User.GetExtra()
	extra["auth_provider_groups"] = identity.GetAuthProviderGroupIDs()

	resp.User = &user.DefaultInfo{
		Name:   gatewayUser.Username,
		UID:    fmt.Sprintf("%d", gatewayUser.ID),
		Extra:  extra,
		Groups: append(resp.User.GetGroups(), gatewayUser.Role.Groups()...),
	}
	return resp, true, nil
}
