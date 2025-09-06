package client

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"

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

	identity := &types.Identity{
		Email:                 firstValue(resp.User.GetExtra(), "email"),
		AuthProviderName:      firstValue(resp.User.GetExtra(), "auth_provider_name"),
		AuthProviderNamespace: firstValue(resp.User.GetExtra(), "auth_provider_namespace"),
		ProviderUsername:      resp.User.GetName(),
		ProviderUserID:        resp.User.GetUID(),
	}

	// Attempt to extract the Obot user ID from the extra claims.
	// If present, this indicates MCP token authentication is being used.
	// It's important to set the user ID value before calling EnsureIdentity so that the token's
	// identity can be matched to an existing user. Not setting this value will cause authentication
	// to fail.
	if userID := firstValue(resp.User.GetExtra(), "obot:userID"); userID != "" {
		id, err := strconv.ParseUint(userID, 10, 64)
		if err != nil {
			return nil, false, fmt.Errorf("failed to parse user id: %w", err)
		}

		identity.UserID = uint(id)
	}

	gatewayUser, err := u.client.EnsureIdentity(req.Context(), identity, req.Header.Get("X-Obot-User-Timezone"))
	if err != nil {
		return nil, false, err
	}

	groups := resp.User.GetGroups()
	if gatewayUser.Role == types2.RoleAdmin && !slices.Contains(groups, authz.AdminGroup) {
		groups = append(groups, authz.AdminGroup)
	}

	extra := resp.User.GetExtra()
	extra["auth_provider_groups"] = identity.GetAuthProviderGroupIDs()

	resp.User = &user.DefaultInfo{
		Name:   gatewayUser.Username,
		UID:    fmt.Sprintf("%d", gatewayUser.ID),
		Extra:  extra,
		Groups: append(groups, authz.AuthenticatedGroup),
	}
	return resp, true, nil
}
