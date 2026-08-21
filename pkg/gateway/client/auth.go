package client

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/utils"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

// UserLimit describes the maximum number of users an installation may have.
// Maximum is ignored when Unlimited is true.
type UserLimit struct {
	Maximum   int64
	Unlimited bool
}

// UserLimitProvider resolves the current license-derived user limit.
type UserLimitProvider interface {
	UserLimit(context.Context) (UserLimit, error)
}

type UserDecorator struct {
	next              authenticator.Request
	client            *Client
	userLimitProvider UserLimitProvider
}

func NewUserDecorator(next authenticator.Request, client *Client, userLimitProvider UserLimitProvider) *UserDecorator {
	return &UserDecorator{
		next:              next,
		client:            client,
		userLimitProvider: userLimitProvider,
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
		gatewayUser  *types.User
		authGroupIDs []string
	)
	if authProviderNamespace, authProviderName := utils.FirstSet(resp.User.GetExtra()["auth_provider_namespace"]...), utils.FirstSet(resp.User.GetExtra()["auth_provider_name"]...); authProviderNamespace != "" && authProviderName != "" {
		identity := &types.Identity{
			Email:                 utils.FirstSet(resp.User.GetExtra()["email"]...),
			AuthProviderName:      authProviderName,
			AuthProviderNamespace: authProviderNamespace,
			ProviderUsername:      resp.User.GetName(),
			ProviderUserID:        resp.User.GetUID(),
		}

		userLimit, err := u.resolveUserLimit(req.Context())
		if err != nil {
			return nil, false, err
		}

		gatewayUser, err = u.client.EnsureIdentity(req.Context(), identity, req.Header.Get("X-Obot-User-Timezone"), userLimit)
		if err != nil {
			return nil, false, err
		}

		authGroupIDs = identity.GetAuthProviderGroupIDs()
	} else {
		return nil, false, nil
	}

	extra := resp.User.GetExtra()
	extra["auth_provider_groups"] = authGroupIDs

	// Resolve effective role by merging individual + group roles
	effectiveRole, err := u.client.ResolveUserEffectiveRole(req.Context(), gatewayUser, authGroupIDs)
	if err != nil {
		// Log error but don't fail authentication - fall back to individual role
		slog.Warn("failed to resolve effective role for user", "userID", gatewayUser.ID, "error", err)
		effectiveRole = gatewayUser.Role
	}

	resp.User = &user.DefaultInfo{
		Name:   gatewayUser.Username,
		UID:    fmt.Sprintf("%d", gatewayUser.ID),
		Extra:  extra,
		Groups: effectiveRole.Groups(),
	}
	return resp, true, nil
}

func (u UserDecorator) resolveUserLimit(ctx context.Context) (UserLimit, error) {
	userLimit, err := u.userLimitProvider.UserLimit(ctx)
	if err != nil {
		return UserLimit{}, fmt.Errorf("failed to resolve user limit: %w", err)
	}

	if !userLimit.Unlimited && userLimit.Maximum <= 0 {
		return UserLimit{}, fmt.Errorf("invalid user limit %d", userLimit.Maximum)
	}

	return userLimit, nil
}
