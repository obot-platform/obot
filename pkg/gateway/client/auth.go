package client

import (
	"fmt"
	"net/http"
	"strconv"

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

	extra := resp.User.GetExtra()
	authProviderNamespace := auth.FirstExtraValue(extra, "auth_provider_namespace")
	authProviderName := auth.FirstExtraValue(extra, "auth_provider_name")
	if authProviderNamespace == "" || authProviderName == "" {
		return nil, false, nil
	}

	var emailVerified *bool
	if raw := auth.FirstExtraValue(extra, "auth_provider_email_verified"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, false, fmt.Errorf("invalid auth_provider_email_verified value %q: %w", raw, err)
		}
		emailVerified = &parsed
	}

	identity := &types.Identity{
		Email:                 auth.FirstExtraValue(extra, "email"),
		AuthProviderName:      authProviderName,
		AuthProviderNamespace: authProviderNamespace,
		ProviderUsername:      resp.User.GetName(),
		ProviderUserID:        resp.User.GetUID(),
		ProviderIssuer:        auth.FirstExtraValue(extra, "auth_provider_issuer"),
		ProviderEmailVerified: emailVerified,
	}
	gatewayUser, err := u.client.EnsureIdentity(req.Context(), identity, req.Header.Get("X-Obot-User-Timezone"))
	if err != nil {
		return nil, false, err
	}

	authGroupIDs := identity.GetAuthProviderGroupIDs()
	extra["auth_provider_groups"] = authGroupIDs

	// Resolve effective role by merging individual + group roles
	effectiveRole, err := u.client.ResolveUserEffectiveRole(req.Context(), gatewayUser, authGroupIDs)
	if err != nil {
		// Log error but don't fail authentication - fall back to individual role
		log.Warnf("failed to resolve effective role for user with ID %d: %s", gatewayUser.ID, err.Error())
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
