package client

import (
	"fmt"
	"net/http"
	"slices"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authz"
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

	gatewayUser, err := u.client.EnsureIdentity(req.Context(), &types.Identity{
		Email:            firstValue(resp.User.GetExtra(), "email"),
		AuthProviderID:   uint(firstValueAsInt(resp.User.GetExtra(), "auth_provider_id")),
		ProviderUsername: resp.User.GetName(),
	}, req.Header.Get("X-Obot-User-Timezone"))
	if err != nil {
		return nil, false, err
	}

	groups := resp.User.GetGroups()
	if gatewayUser.Role == types2.RoleAdmin && !slices.Contains(groups, authz.AdminGroup) {
		groups = append(groups, authz.AdminGroup)
	}

	resp.User = &user.DefaultInfo{
		Name:   gatewayUser.Username,
		UID:    fmt.Sprintf("%d", gatewayUser.ID),
		Extra:  resp.User.GetExtra(),
		Groups: append(groups, authz.AuthenticatedGroup),
	}
	return resp, true, nil
}
