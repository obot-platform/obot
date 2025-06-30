package client

import (
	"fmt"
	"net/http"
	"slices"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authz"
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

	var (
		authProviderName      = firstValue(resp.User.GetExtra(), "auth_provider_name")
		authProviderNamespace = firstValue(resp.User.GetExtra(), "auth_provider_namespace")
		groupInfos            = auth.GroupInfosFromContext(req.Context())
		authProviderGroups    = make([]types.Group, 0, len(groupInfos))
	)
	if authProviderName != "" && authProviderNamespace != "" {
		for _, groupInfo := range groupInfos {
			authProviderGroups = append(authProviderGroups, types.Group{
				ID:                    groupInfo.ID,
				AuthProviderName:      authProviderName,
				AuthProviderNamespace: authProviderNamespace,
				Name:                  groupInfo.Name,
				IconURL:               groupInfo.IconURL,
			})
		}
	}

	gatewayUser, err := u.client.EnsureIdentity(req.Context(), &types.Identity{
		Email:                 firstValue(resp.User.GetExtra(), "email"),
		AuthProviderName:      firstValue(resp.User.GetExtra(), "auth_provider_name"),
		AuthProviderNamespace: firstValue(resp.User.GetExtra(), "auth_provider_namespace"),
		AuthProviderGroups:    authProviderGroups,
		ProviderUsername:      resp.User.GetName(),
		ProviderUserID:        resp.User.GetUID(),
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
