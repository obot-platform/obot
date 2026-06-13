package oidcjwt

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	genericOAuthAuthProviderName = "generic-oauth-auth-provider"
	jwtRolesExtraKey             = "oidcjwt_roles"
)

type Authenticator struct {
	cfg      Config
	verifier *Verifier
}

func NewAuthenticator(cfg Config, verifier *Verifier) *Authenticator {
	cfg.IssuerURL = NormalizeIssuer(cfg.IssuerURL)
	return &Authenticator{cfg: cfg, verifier: verifier}
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

	extra := map[string][]string{
		"email":                   {claims.Email},
		"auth_provider_name":      {genericOAuthAuthProviderName},
		"auth_provider_namespace": {system.DefaultNamespace},
		"auth_provider_issuer":    {NormalizeIssuer(claims.Issuer)},
		"auth_provider_user_id":   {claims.Subject},
		jwtRolesExtraKey:          claims.Roles,
	}
	if claims.EmailVerified != nil {
		extra["auth_provider_email_verified"] = []string{fmt.Sprintf("%t", *claims.EmailVerified)}
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:  providerUsername(claims),
			UID:   claims.Subject,
			Extra: extra,
		},
	}, true, nil
}

func providerUsername(claims Claims) string {
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
	return username
}

func bearerToken(req *http.Request) string {
	h := req.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
}
