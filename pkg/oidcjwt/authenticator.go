package oidcjwt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const genericOAuthAuthProviderName = "generic-oauth-auth-provider"

type IdentityResolver interface {
	ResolveOrCreate(ctx context.Context, id *gwtypes.Identity, timezone string) (*gwtypes.User, error)
}

type Authenticator struct {
	cfg      Config
	verifier *Verifier
	identity IdentityResolver
}

func NewAuthenticator(cfg Config, verifier *Verifier, identity IdentityResolver) *Authenticator {
	cfg.IssuerURL = NormalizeIssuer(cfg.IssuerURL)
	return &Authenticator{cfg: cfg, verifier: verifier, identity: identity}
}

func (a *Authenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	if !a.cfg.Enabled() || a.verifier == nil {
		return nil, false, nil
	}

	raw := bearerToken(req)
	if raw == "" {
		return nil, false, nil
	}

	claims, err := a.verifier.Verify(req.Context(), raw)
	if errors.Is(err, ErrNotMyToken) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("oidcjwt: %w", err)
	}
	if !claims.Eligible {
		return nil, false, errors.New("oidcjwt: eligibility claim missing or false")
	}
	if a.identity == nil {
		return nil, false, errors.New("oidcjwt: identity resolver not configured")
	}

	id := buildIdentity(claims)
	gwUser, err := a.identity.ResolveOrCreate(req.Context(), id, req.Header.Get("X-Obot-User-Timezone"))
	if err != nil {
		return nil, false, fmt.Errorf("oidcjwt: identity resolve: %w", err)
	}

	extra := map[string][]string{
		"email":                   {gwUser.Email},
		"auth_provider_name":      {genericOAuthAuthProviderName},
		"auth_provider_namespace": {system.DefaultNamespace},
		"auth_provider_issuer":    {claims.Issuer},
		"auth_provider_user_id":   {id.ProviderUserID},
	}
	if claims.EmailVerified != nil {
		extra["auth_provider_email_verified"] = []string{fmt.Sprintf("%t", *claims.EmailVerified)}
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   gwUser.Username,
			UID:    fmt.Sprintf("%d", gwUser.ID),
			Groups: deriveGroups(claims.Roles, a.cfg.AdminRoles),
			Extra:  extra,
		},
	}, true, nil
}

func buildIdentity(claims Claims) *gwtypes.Identity {
	username := claims.PreferredUsername
	if username == "" {
		username = claims.Name
	}
	if username == "" {
		username = claims.Email
	}
	if username == "" {
		username = claims.Subject
	}

	return &gwtypes.Identity{
		AuthProviderName:      genericOAuthAuthProviderName,
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      username,
		ProviderUserID:        "iss:" + claims.Issuer + "\x00sub:" + claims.Subject,
		ProviderIssuer:        claims.Issuer,
		ProviderEmailVerified: claims.EmailVerified,
		Email:                 claims.Email,
		IconURL:               claims.Picture,
	}
}

func deriveGroups(jwtRoles, adminRoles []string) []string {
	adminSet := make(map[string]struct{}, len(adminRoles))
	for _, role := range adminRoles {
		adminSet[role] = struct{}{}
	}
	for _, role := range jwtRoles {
		if _, ok := adminSet[role]; ok {
			return []string{types.GroupAdmin, types.GroupOwner, types.GroupAuthenticated}
		}
	}
	return []string{types.GroupAuthenticated}
}

func bearerToken(req *http.Request) string {
	h := req.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}
