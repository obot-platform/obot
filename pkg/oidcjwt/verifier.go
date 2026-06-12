package oidcjwt

import (
	"context"
	"errors"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v5"
)

var ErrNotMyToken = errors.New("oidcjwt: token not for this authenticator")

type Claims struct {
	Issuer  string
	Subject string

	Eligible bool
	Roles    []string

	Email             string
	EmailVerified     *bool
	PreferredUsername string
	Name              string
	Picture           string
}

type Verifier struct {
	cfg      Config
	verifier *oidc.IDTokenVerifier
}

func NewVerifier(ctx context.Context, cfg Config) (*Verifier, error) {
	cfg.IssuerURL = NormalizeIssuer(cfg.IssuerURL)
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("oidcjwt: oidc discovery: %w", err)
	}
	verifier := provider.Verifier(&oidc.Config{
		ClientID:             cfg.Audience,
		SupportedSigningAlgs: []string{"RS256"},
	})
	return &Verifier{cfg: cfg, verifier: verifier}, nil
}

func (v *Verifier) Verify(ctx context.Context, raw string) (Claims, error) {
	parser := jwt.NewParser()
	parsed, _, err := parser.ParseUnverified(raw, jwt.MapClaims{})
	if err != nil {
		return Claims{}, ErrNotMyToken
	}
	mc, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return Claims{}, ErrNotMyToken
	}
	iss, _ := mc["iss"].(string)
	if iss != v.cfg.IssuerURL {
		return Claims{}, ErrNotMyToken
	}

	idToken, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return Claims{}, fmt.Errorf("oidcjwt: verify: %w", err)
	}

	var custom struct {
		Email             string `json:"email"`
		EmailVerified     *bool  `json:"email_verified,omitempty"`
		PreferredUsername string `json:"preferred_username,omitempty"`
		Name              string `json:"name,omitempty"`
		Picture           string `json:"picture,omitempty"`
	}
	_ = idToken.Claims(&custom)

	return Claims{
		Issuer:            idToken.Issuer,
		Subject:           idToken.Subject,
		Eligible:          readEligibility(mc, v.cfg.EligibilityClaimName),
		Roles:             readRoles(mc, v.cfg.RolesClaimName),
		Email:             custom.Email,
		EmailVerified:     custom.EmailVerified,
		PreferredUsername: custom.PreferredUsername,
		Name:              custom.Name,
		Picture:           custom.Picture,
	}, nil
}

func readEligibility(mc jwt.MapClaims, name string) bool {
	if name == "" {
		return false
	}
	switch v := mc[name].(type) {
	case bool:
		return v
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				return true
			}
		}
	}
	return false
}

func readRoles(mc jwt.MapClaims, name string) []string {
	if name == "" {
		return nil
	}
	raw, ok := mc[name].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if s, ok := r.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
