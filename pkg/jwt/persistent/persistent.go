package persistent

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"
	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	keyEnvVar = "JWK_KEY"
	// Bound API-key display metadata before JWT encoding expands it into an HTTP header.
	maxAPIKeyNameCharacters = 512

	// DelegatedTokenTTL is how long a token with which an agent acts on behalf of a user lasts (NewDelegatedToken),
	// unless its minter asks for less.
	DelegatedTokenTTL = 15 * time.Minute

	// actorClaim and delegationIDClaim are the claims that only a token with which an agent acts on behalf of a user
	// carries (TokenContext.Actor and TokenContext.DelegationID).
	actorClaim        = "act"
	delegationIDClaim = "obot_delegation_id"
)

var (
	// ErrDelegatedToken refuses a token with which an agent acts on behalf of a user wherever such tokens are not
	// accepted: everywhere but DecodeDelegatedToken.
	ErrDelegatedToken = errors.New("a token that acts on behalf of a user is not accepted here")

	// errNotDelegatedToken refuses any other token where only tokens that act on behalf of a user are read.
	errNotDelegatedToken = errors.New("not a token that acts on behalf of a user")

	// errMalformedDelegatedToken refuses a token that acts on behalf of a user that NewDelegatedToken would not issue.
	errMalformedDelegatedToken = errors.New("malformed token that acts on behalf of a user")

	// agentTokenGroups are the only groups a token issued for an agent may carry. Tokens are only issued to agents
	// for Obot's internal MCP connections, such as a composite vMCP's loopback to its components, and an agent has
	// no role, so no token grants an agent any other route.
	agentTokenGroups = []string{apitypes.GroupMCP, apitypes.GroupCompositeMCP, apitypes.GroupAuthenticated}
)

type TokenService struct {
	lock          sync.RWMutex
	privateKey    ed25519.PrivateKey
	jwks          json.RawMessage
	gatewayClient *client.Client
	serverURL     string
}

type TokenContext struct {
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	IssuedAt  Time   `json:"iat"`
	ExpiresAt Time   `json:"exp"`
	// UserID is the subject: the storage encoding of the principal the token is for (principal.Principal.String),
	// a user's decimal ID or "agent:<id>".
	UserID string `json:"sub"`
	// PrincipalKind is the kind of the subject, which NewToken records. Tokens issued before it was recorded have
	// none, and are users'. A token whose subject is not of this kind authenticates no one.
	PrincipalKind         string `json:"obot_principal_kind,omitempty"`
	UserName              string `json:"name"`
	UserEmail             string `json:"email"`
	OAuthScope            string `json:"scope"`
	OAuthClientID         string `json:"client_id,omitempty"`
	OAuthConnectionID     string `json:"obot_connection_id,omitempty"`
	Picture               string `json:"picture"`
	UserGroups            StringSlice
	AuthProviderName      string
	AuthProviderNamespace string
	AuthProviderUserID    string

	// Actor is set only on a token with which an agent acts on behalf of a user (NewDelegatedToken): the user is its
	// subject (UserID), and the agent is its actor (RFC 8693). Because a check that reads only the subject would take
	// such a token for the user's own, every decoder refuses it except DecodeDelegatedToken, which callers opt in to.
	Actor *TokenActor `json:"act,omitempty"`
	// DelegationID is the delegation that a token acting on behalf of a user was minted from.
	DelegationID string `json:"obot_delegation_id,omitempty"`

	// Non-secret attribution retained across authenticated composite loopback calls. A token acting on behalf of a
	// user records the ID of the agent's key that minted it here as well.
	APIKeyID      string `json:"api_key_id,omitempty"`
	APIKeyName    string `json:"api_key_name,omitempty"`
	APIKeyRawName string `json:"api_key_raw_name,omitempty"`

	MCPID            string
	AuthorizedMCPIDs StringSlice

	// This is used for requesting community license
	InstallationID string `json:"installation_id,omitempty"`

	// The following fields are for runs
	Namespace     string
	ModelProvider string
	Model         string
}

// TokenActor is the actor (RFC 8693 "act") claim of a token with which an agent acts on behalf of a user.
type TokenActor struct {
	// Subject is the storage encoding of the agent, "agent:<id>" (principal.Principal.String).
	Subject string `json:"sub"`
}

// DelegatedToken is what a token with which an agent acts on behalf of a user establishes (NewDelegatedToken).
type DelegatedToken struct {
	// AgentID is the agent that acts, from the token's actor.
	AgentID uint
	// UserID is the user the agent acts for, from the token's subject.
	UserID uint
	// DelegationID is the delegation the token was minted from.
	DelegationID string
	// APIKeyID is the agent's key that minted the token.
	APIKeyID uint
	// ExpiresAt is when the token expires.
	ExpiresAt time.Time
	// Loopback is set only on a token of a composite vMCP's loopback that acts for the agent on behalf of the user
	// (NewDelegatedLoopbackToken): what the loopback carries beyond the delegation. It is nil on the token an agent is
	// given (NewDelegatedToken).
	Loopback *DelegatedLoopback
}

// DelegatedLoopback is what a token of a composite vMCP's loopback carries for an agent acting on behalf of a user,
// beyond the delegation: the claims of the composite loopback token an agent acting for itself gets
// (TokenContext), which only Obot itself issues, for its own connections to the composite and its components.
type DelegatedLoopback struct {
	// Audience is the composite's connect URL.
	Audience string
	// Groups are the loopback's groups. They always include types.GroupCompositeMCP, and are only groups that tokens
	// are issued to agents with (agentTokenGroups).
	Groups []string
	// MCPID is the vMCP connection the loopback serves, and AuthorizedMCPIDs its component connections.
	MCPID            string
	AuthorizedMCPIDs []string
	// OAuthConnectionID is the vMCP connection the caller reached, and OAuthClientID the OAuth client it reached it
	// with, if any.
	OAuthConnectionID string
	OAuthClientID     string
	// APIKeyName and APIKeyRawName are the attribution of the minting key.
	APIKeyName    string
	APIKeyRawName string
}

// NewTokenService returns a service that issues and authenticates Obot's own tokens, for users and agents alike.
func NewTokenService(serverURL string, gatewayClient *client.Client) (*TokenService, error) {
	t := &TokenService{
		gatewayClient: gatewayClient,
		serverURL:     serverURL,
	}
	return t, nil
}

// EnsureJWK ensures that the JWK is created and stored. It should only be called in a controller post-start hook which only allows one to be run at a time.
func (t *TokenService) EnsureJWK(ctx context.Context) error {
	// Read the credential, if it exists, then use it.
	cred, err := t.gatewayClient.RevealCredential(ctx, []string{system.JWKCredentialContext}, system.JWKCredentialContext)
	if err != nil && !errors.As(err, &client.CredentialNotFoundError{}) {
		return err
	}

	var configuredKey ed25519.PrivateKey
	if keyData := cred.Secrets[keyEnvVar]; keyData != "" {
		configuredKey, err = base64.StdEncoding.DecodeString(keyData)
		if err != nil {
			return err
		}
	} else {
		// Create a key.
		_, configuredKey, err = ed25519.GenerateKey(nil)
		if err != nil {
			return err
		}
	}

	// Write the key to the JWK Set storage.
	if err := t.gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: system.JWKCredentialContext,
		Name:    system.JWKCredentialContext,
		Secrets: map[string]string{
			keyEnvVar: base64.StdEncoding.EncodeToString(configuredKey),
		},
	}); err != nil {
		return err
	}

	return nil
}

// SetJWK sets the JWK in the database. It should be called after the JWK is created and stored in the GPTScript client.
func (t *TokenService) setJWK(ctx context.Context) error {
	cred, err := t.gatewayClient.RevealCredential(ctx, []string{system.JWKCredentialContext}, system.JWKCredentialContext)
	if err != nil && !errors.As(err, &client.CredentialNotFoundError{}) {
		return err
	}

	value, ok := cred.Secrets[keyEnvVar]
	if !ok || value == "" {
		return fmt.Errorf("JWK not found in credential")
	}

	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return fmt.Errorf("failed to decode JWK: %w", err)
	}

	if err := t.replaceKey(ctx, key); err != nil {
		return err
	}

	return nil
}

func (t *TokenService) ReplaceJWK(req api.Context) error {
	// Create a key.
	_, newKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: system.JWKCredentialContext,
		Name:    system.JWKCredentialContext,
		Secrets: map[string]string{
			keyEnvVar: base64.StdEncoding.EncodeToString(newKey),
		},
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	if err := t.replaceKey(req.Context(), newKey); err != nil {
		return fmt.Errorf("failed to replace key: %w", err)
	}

	return nil
}

func (t TokenContext) GetAudience() (jwt.ClaimStrings, error) {
	if t.Audience == "" {
		return nil, nil
	}
	return jwt.ClaimStrings([]string{t.Audience}), nil
}

// GetExpirationTime implements the Claims interface.
func (t TokenContext) GetExpirationTime() (*jwt.NumericDate, error) {
	return &jwt.NumericDate{Time: t.ExpiresAt.Time}, nil
}

// GetNotBefore implements the Claims interface.
func (t TokenContext) GetNotBefore() (*jwt.NumericDate, error) {
	return nil, nil
}

// GetIssuedAt implements the Claims interface.
func (t TokenContext) GetIssuedAt() (*jwt.NumericDate, error) {
	return &jwt.NumericDate{Time: t.IssuedAt.Time}, nil
}

// GetIssuer implements the Claims interface.
func (t TokenContext) GetIssuer() (string, error) {
	return t.Issuer, nil
}

// GetSubject implements the Claims interface.
func (t TokenContext) GetSubject() (string, error) {
	return t.UserID, nil
}

// Principal returns the principal the token is for: its subject, which must be of the kind its PrincipalKind claim
// names. A token issued before that claim was recorded has none, and is a user's, so such a token can never
// authenticate an agent. It reports false for a subject that is not a principal, or not of the claimed kind, so a
// token is never read as an identity of another kind, such as the user with an agent's numeric ID.
func (t TokenContext) Principal() (principal.Principal, bool) {
	p, err := principal.Parse(t.UserID)
	if err != nil {
		return principal.Principal{}, false
	}
	kind := principal.Kind(t.PrincipalKind)
	if kind == "" {
		kind = principal.KindUser
	}
	if kind != p.Kind {
		return principal.Principal{}, false
	}
	return p, true
}

// RequestToken returns the token a request presents: the Authorization header's bearer value, or else the X-API-Key
// header's value.
func RequestToken(req *http.Request) string {
	if token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer "); token != "" {
		return token
	}
	return req.Header.Get("X-API-Key")
}

// ClaimsAgentToken reports whether token is a JWT whose claims name an agent, by its subject or by its principal kind,
// or that acts on behalf of a user, which only an agent does (claimsDelegation). The claims are read without checking
// the token's signature or expiry: a token is classified by what it claims to be, so that one that is forged, expired
// or signed with a replaced key is refused like any other agent token that fails. It is the one classifier of agent
// tokens, for this guard and for any code that must not treat a refused agent token as a user's, such as the MCP
// gateway's decision to withhold an interactive challenge.
func ClaimsAgentToken(token string) bool {
	claims, ok := unverifiedClaims(token)
	if !ok {
		return false
	}
	subject, _ := claims["sub"].(string)
	// The principal kind claim has the name of the user info extra that records the kind once the token authenticates.
	kind, _ := claims[principal.KindExtra].(string)
	// The subject of an agent's token is its storage encoding, "agent:<id>" (principal.Principal.String). Its prefix
	// is tested rather than the subject parsed, so that a malformed agent subject is classified as an agent's too. A
	// token that acts on behalf of a user has the user as its subject, and is classified by its claims instead.
	return principal.IsAgentKind(kind) || principal.ClaimsAgentOwner(subject) || claimsDelegation(claims)
}

// unverifiedClaims returns the claims of token, a JWT, without checking its signature or expiry. It is only for
// classifying a token by what it claims to be (ClaimsAgentToken, ClaimsDelegatedToken), never for trusting them.
func unverifiedClaims(token string) (jwt.MapClaims, bool) {
	claims := jwt.MapClaims{}
	_, _, err := jwt.NewParser().ParseUnverified(token, claims)
	return claims, err == nil
}

func (t *TokenService) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	token := RequestToken(req)
	if token == "" {
		return nil, false, nil
	}

	tokenContext, err := t.DecodeToken(req.Context(), token)
	if err != nil {
		// A token with which an agent acts on behalf of a user is refused rather than declined, whatever is wrong with
		// it, so that no later authenticator, such as the catch-all of a server that does not authenticate callers or
		// the anonymous one, takes the request as anyone. Its subject is the user, so a valid one would otherwise read
		// as the user's own.
		if ClaimsDelegatedToken(token) {
			return nil, false, ErrDelegatedToken
		}
		return nil, false, nil
	}

	// Tokens for anything other than a user or an agent are not authenticated here.
	owner, ok := tokenContext.Principal()
	if !ok {
		return nil, false, nil
	}

	// What the token itself grants, for either kind of principal.
	extra := map[string][]string{
		"mcp_id":              {tokenContext.MCPID},
		"resource":            {tokenContext.Audience},
		"oauthScope":          {tokenContext.OAuthScope},
		"oauth_client_id":     {tokenContext.OAuthClientID},
		"oauth_connection_id": {tokenContext.OAuthConnectionID},
		"authorized_mcp_ids":  tokenContext.AuthorizedMCPIDs,
	}
	if tokenContext.MCPID != "" {
		extra["authorized_mcp_ids"] = append(extra["authorized_mcp_ids"], tokenContext.MCPID)
	}

	// Only signed internal composite tokens propagate the originating API key.
	if slices.Contains(tokenContext.UserGroups, apitypes.GroupCompositeMCP) && tokenContext.MCPID != "" {
		extra[principal.APIKeyIDExtra] = []string{tokenContext.APIKeyID}
		extra[principal.APIKeyNameExtra] = []string{tokenContext.APIKeyName}
		extra[principal.APIKeyRawNameExtra] = []string{tokenContext.APIKeyRawName}
	}

	if agentID, ok := owner.AgentID(); ok {
		return t.authenticateAgent(req.Context(), tokenContext, agentID, extra)
	}
	userID, ok := owner.UserID()
	if !ok {
		return nil, false, nil
	}

	extra["email"] = []string{tokenContext.UserEmail}
	extra["auth_provider_name"] = []string{tokenContext.AuthProviderName}
	extra["auth_provider_namespace"] = []string{tokenContext.AuthProviderNamespace}
	groups := tokenContext.UserGroups
	extra["obot_groups"] = slices.Clone(groups)

	// Read the user and their auth provider groups in one transaction. The role is recomputed from them on every
	// use, so an old token cannot keep a removed group's role, and the user's lifecycle status comes from the
	// same read. IdentityInfo is not used because this principal carries only role groups in obot_groups.
	gatewayUser, authGroupIDs, err := t.gatewayClient.UserByIDWithEffectiveRole(req.Context(), userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// The token outlived its user. The admission check denies it.
		principal.RecordPrincipal(extra, principal.KindUser, apitypes.UserStatusDeleted)
	} else if err != nil {
		return nil, false, &client.UserAccessLookupError{
			UserID: userID,
			Err:    err,
		}
	} else {
		extra["auth_provider_groups"] = authGroupIDs
		extra["obot_groups"] = gatewayUser.Role.RoleGroups()
		principal.RecordPrincipal(extra, principal.KindUser, gatewayUser.Status())
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			UID:    tokenContext.UserID,
			Name:   tokenContext.UserName,
			Groups: groups,
			Extra:  extra,
		},
	}, true, nil
}

// HasLocalIssuer reads the unverified issuer only to prevent forwarding Obot
// credentials to an external provider. It must not be used to authenticate a token.
func (t *TokenService) HasLocalIssuer(token string) bool {
	if len(token) >= len("Bearer ") && strings.EqualFold(token[:len("Bearer ")], "Bearer ") {
		token = token[len("Bearer "):]
	}

	var claims jwt.RegisteredClaims
	_, _, err := jwt.NewParser().ParseUnverified(token, &claims)
	return err == nil && claims.Issuer != "" && claims.Issuer == t.serverURL
}

// authenticateAgent returns the principal of the agent that a token was issued for, with extra, what the token
// grants. It is built as the API key authenticator builds an agent's (client.AgentUserInfo), from the agent's current
// row: never with the token's name or email, and never with obot_groups or auth_provider_groups, which only users
// have. The agent is read on every use, so a token issued before the agent was disabled or deleted is denied from its
// next use on.
func (t *TokenService) authenticateAgent(ctx context.Context, tokenContext *TokenContext, agentID uint, extra map[string][]string) (*authenticator.Response, bool, error) {
	// NewToken issues agents no other group, so a token with another one was not issued by it, and is not honored.
	if _, refused := refusedAgentTokenGroup(tokenContext.UserGroups); refused {
		return nil, false, nil
	}

	// The agent's row is read directly, since its principal is built here with the token's groups and extra. An agent
	// that is not active is denied here rather than left to the admission check, as the API key authenticator denies
	// one, and so is a token that outlived its agent.
	agent, err := t.gatewayClient.ActiveAgent(ctx, agentID)
	if err != nil {
		return nil, false, err
	}

	return &authenticator.Response{
		User: client.AgentUserInfo(agent, slices.Clone([]string(tokenContext.UserGroups)), extra),
	}, true, nil
}

// DecodeToken returns the claims of one of Obot's own tokens, once its signature, issuer, expiry and audience are
// verified. It refuses a token with which an agent acts on behalf of a user (ErrDelegatedToken): only
// DecodeDelegatedToken reads one.
func (t *TokenService) DecodeToken(ctx context.Context, token string) (*TokenContext, error) {
	claims, err := t.verifiedClaims(ctx, token, false)
	if err != nil {
		return nil, err
	}
	return tokenContextFromClaims(claims), nil
}

// DecodeDelegatedToken returns what a token with which an agent acts on behalf of a user establishes, once the token is
// verified as DecodeToken verifies one. It is the only decoder that reads such a token, and it reads no other: a
// token that does not name an agent as its actor, a user as its subject, a delegation, the agent's key that minted it
// and its expiry is refused.
//
// The token's user is not the caller. A caller that accepts these tokens acts as the agent, on behalf of the user
// (principal.RecordOnBehalfOf), and must check on every use that the delegation, the agent, the user and the key are
// still valid.
func (t *TokenService) DecodeDelegatedToken(ctx context.Context, token string) (*DelegatedToken, error) {
	claims, err := t.verifiedClaims(ctx, token, true)
	if err != nil {
		return nil, err
	}
	tokenContext := tokenContextFromClaims(claims)

	if tokenContext.Actor == nil {
		return nil, fmt.Errorf("%w: no actor", errMalformedDelegatedToken)
	}
	agent, err := principal.Parse(tokenContext.Actor.Subject)
	if err != nil {
		return nil, fmt.Errorf("%w: actor: %w", errMalformedDelegatedToken, err)
	}
	agentID, ok := agent.AgentID()
	if !ok || agentID == 0 {
		return nil, fmt.Errorf("%w: actor %q is not an agent", errMalformedDelegatedToken, tokenContext.Actor.Subject)
	}
	// The subject must be a user, recorded as one: a token never acts for an agent, or for anything that is not a user.
	user, isPrincipal := tokenContext.Principal()
	userID, isUser := user.UserID()
	if !isPrincipal || tokenContext.PrincipalKind != string(principal.KindUser) || !isUser || userID == 0 {
		return nil, fmt.Errorf("%w: subject %q is not a user", errMalformedDelegatedToken, tokenContext.UserID)
	}
	if tokenContext.DelegationID == "" {
		return nil, fmt.Errorf("%w: no delegation", errMalformedDelegatedToken)
	}
	apiKeyID, err := strconv.ParseUint(tokenContext.APIKeyID, 10, 0)
	if err != nil || apiKeyID == 0 {
		return nil, fmt.Errorf("%w: no minting key", errMalformedDelegatedToken)
	}
	// The parser checks an expiry only when the token has one, and this kind of token always has one.
	if _, ok := claims["exp"]; !ok || tokenContext.ExpiresAt.IsZero() {
		return nil, fmt.Errorf("%w: no expiry", errMalformedDelegatedToken)
	}

	delegated := &DelegatedToken{
		AgentID:      agentID,
		UserID:       userID,
		DelegationID: tokenContext.DelegationID,
		APIKeyID:     uint(apiKeyID),
		ExpiresAt:    tokenContext.ExpiresAt.Time,
	}
	// The token an agent is given carries no groups: what it grants comes from the minting key. Only a composite
	// loopback's carries any, and then exactly what NewDelegatedLoopbackToken issues.
	if len(tokenContext.UserGroups) > 0 || tokenContext.MCPID != "" || len(tokenContext.AuthorizedMCPIDs) > 0 {
		loopback := DelegatedLoopback{
			Audience:          tokenContext.Audience,
			Groups:            slices.Clone([]string(tokenContext.UserGroups)),
			MCPID:             tokenContext.MCPID,
			AuthorizedMCPIDs:  slices.Clone([]string(tokenContext.AuthorizedMCPIDs)),
			OAuthConnectionID: tokenContext.OAuthConnectionID,
			OAuthClientID:     tokenContext.OAuthClientID,
			APIKeyName:        tokenContext.APIKeyName,
			APIKeyRawName:     tokenContext.APIKeyRawName,
		}
		if err := checkDelegatedLoopback(loopback); err != nil {
			return nil, fmt.Errorf("%w: %w", errMalformedDelegatedToken, err)
		}
		delegated.Loopback = &loopback
	}
	return delegated, nil
}

// verifiedClaims returns the claims of one of Obot's own tokens, once its signature, issuer, expiry and audience are
// verified. It is the one step that every decoder of these tokens takes.
//
// delegated says whether its caller reads tokens with which an agent acts on behalf of a user (claimsDelegation): a
// token is refused unless it is the kind its caller reads. Only DecodeDelegatedToken reads that kind, so any other
// decoder, including one added later, refuses those tokens without a check of its own.
func (t *TokenService) verifiedClaims(ctx context.Context, token string, delegated bool) (jwt.MapClaims, error) {
	t.lock.RLock()
	privateKey := t.privateKey
	t.lock.RUnlock()

	if privateKey == nil {
		if err := t.setJWK(ctx); err != nil {
			return nil, err
		}

		t.lock.RLock()
		privateKey = t.privateKey
		t.lock.RUnlock()
	}

	tk, err := jwt.Parse(token, func(*jwt.Token) (any, error) {
		t.lock.RLock()
		defer t.lock.RUnlock()

		return privateKey.Public(), nil
	}, jwt.WithIssuer(t.serverURL))
	if err != nil {
		return nil, err
	}

	claims, ok := tk.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("unexpected claims type %T", tk.Claims)
	}

	audiences, err := claims.GetAudience()
	if err != nil {
		return nil, err
	}

	if len(audiences) == 0 || audiences[0] == "" {
		return nil, fmt.Errorf("no audience")
	}

	switch isDelegated := claimsDelegation(claims); {
	case isDelegated && !delegated:
		return nil, ErrDelegatedToken
	case !isDelegated && delegated:
		return nil, errNotDelegatedToken
	}

	return claims, nil
}

// tokenContextFromClaims reads the claims of a verified token.
func tokenContextFromClaims(claims jwt.MapClaims) *TokenContext {
	var groups []string
	if userGroups, ok := claims["UserGroups"].(string); ok && userGroups != "" {
		groups = strings.Split(userGroups, ",")
		groups = slices.DeleteFunc(groups, func(s string) bool { return s == "" })
	}

	var authorizedMCPIDs StringSlice
	if mcpIDs, ok := claims["AuthorizedMCPIDs"].(string); ok && mcpIDs != "" {
		authorizedMCPIDs = StringSlice(strings.Split(mcpIDs, ","))
	}

	var issuedAt, expiresAt time.Time
	if iat, ok := claims["iat"].(float64); ok {
		issuedAt = time.Unix(int64(iat), 0)
	}

	if exp, ok := claims["exp"].(float64); ok {
		expiresAt = time.Unix(int64(exp), 0)
	}

	getStringClaim := func(keys ...string) string {
		for _, key := range keys {
			if val, ok := claims[key].(string); ok {
				return val
			}
		}

		return ""
	}

	var actor *TokenActor
	if act, ok := claims[actorClaim].(map[string]any); ok {
		subject, _ := act["sub"].(string)
		actor = &TokenActor{Subject: subject}
	}

	return &TokenContext{
		IssuedAt:              NewTime(issuedAt),
		ExpiresAt:             NewTime(expiresAt),
		UserGroups:            groups,
		Audience:              getStringClaim("aud"),
		UserID:                getStringClaim("sub"),
		PrincipalKind:         getStringClaim(principal.KindExtra),
		Actor:                 actor,
		DelegationID:          getStringClaim(delegationIDClaim),
		Picture:               getStringClaim("picture"),
		OAuthScope:            getStringClaim("scope"),
		OAuthClientID:         getStringClaim("client_id"),
		OAuthConnectionID:     getStringClaim("obot_connection_id"),
		AuthProviderName:      getStringClaim("AuthProviderName"),
		AuthProviderNamespace: getStringClaim("AuthProviderNamespace"),
		AuthProviderUserID:    getStringClaim("AuthProviderUserID"),
		MCPID:                 getStringClaim("MCPID"),
		APIKeyID:              getStringClaim("api_key_id"),
		APIKeyName:            getStringClaim("api_key_name"),
		APIKeyRawName:         getStringClaim("api_key_raw_name"),
		AuthorizedMCPIDs:      authorizedMCPIDs,
		Namespace:             getStringClaim("Namespace"),
		ModelProvider:         getStringClaim("ModelProvider"),
		Model:                 getStringClaim("Model"),

		// These fields were the latter names and changed the former.
		// This makes this backwards compatible with older tokens.
		UserName:  getStringClaim("name", "UserName"),
		UserEmail: getStringClaim("email", "UserEmail"),
	}
}

// claimsDelegation reports whether claims are those of a token with which an agent acts on behalf of a user: they
// carry an actor or a delegation, whatever their values.
func claimsDelegation(claims jwt.MapClaims) bool {
	_, hasActor := claims[actorClaim]
	_, hasDelegation := claims[delegationIDClaim]
	return hasActor || hasDelegation
}

// ClaimsDelegatedToken reports whether token is a JWT whose claims are those of a token with which an agent acts on
// behalf of a user (claimsDelegation). Like ClaimsAgentToken, it reads the claims without checking the token's
// signature or expiry, so that such a token is refused whatever is wrong with it.
func ClaimsDelegatedToken(token string) bool {
	claims, ok := unverifiedClaims(token)
	return ok && claimsDelegation(claims)
}

// NewToken issues one of Obot's own tokens with the claims of context. It never issues a token with which an agent
// acts on behalf of a user: such a token's subject is the user, and only NewDelegatedToken issues one.
func (t *TokenService) NewToken(ctx context.Context, context TokenContext) (*jwt.Token, string, error) {
	if context.Audience == "" {
		return nil, "", fmt.Errorf("audience is required")
	}

	if context.Actor != nil || context.DelegationID != "" {
		return nil, "", errors.New("a token that acts on behalf of a user is only issued by NewDelegatedToken")
	}

	// The owner of a vMCP's preconfigured OAuth grants stands for a vMCP's grants, never for a caller, so no token is
	// issued for it, even one that would never authenticate. The owner of a user's setup through an agent is likewise no
	// caller.
	if principal.IsPreconfiguredOAuthOwner(context.UserID) {
		return nil, "", fmt.Errorf("token subject %q is a vMCP preconfigured OAuth grant owner", context.UserID)
	}
	if principal.IsDelegatedOwner(context.UserID) {
		return nil, "", fmt.Errorf("token subject %q is the owner of a user's setup through an agent", context.UserID)
	}

	// The token records the kind of its subject, so that it authenticates only as a principal of that kind. A
	// subject that is not a principal, such as a system principal, has no kind, and is issued a token as before.
	owner, parseErr := principal.Parse(context.UserID)
	isPrincipal := parseErr == nil
	switch {
	case isPrincipal && context.PrincipalKind != "" && principal.Kind(context.PrincipalKind) != owner.Kind:
		return nil, "", fmt.Errorf("token subject %q is not of kind %q", context.UserID, context.PrincipalKind)
	case isPrincipal:
		context.PrincipalKind = string(owner.Kind)
	case context.PrincipalKind != "":
		return nil, "", fmt.Errorf("token subject %q of kind %q is not a principal", context.UserID, context.PrincipalKind)
	}

	// A token is never issued for a disabled or deleted user or agent, including to background work acting for them. A
	// request that the admission check let through has read the status of the principal it acts for already, so a
	// token that it mints for that principal is not checked again.
	if agentID, ok := owner.AgentID(); ok {
		if err := t.checkAgentToken(ctx, context, agentID); err != nil {
			return nil, "", err
		}
	} else if userID, ok := owner.UserID(); ok && t.gatewayClient != nil && !principal.AdmittedActiveUser(ctx, context.UserID) {
		if err := t.gatewayClient.CheckCredentialOwner(ctx, userID); err != nil {
			return nil, "", err
		}
	}

	if strings.HasPrefix(context.Picture, "data:") {
		// Don't store the picture in the token if it is a base64 encoded image
		context.Picture = ""
	}
	// Both initial composite tokens and background refreshes pass through here.
	// Header-injection limits run after the loopback hop, so they cannot bound
	// the Authorization header that carries this token.
	if utf8.RuneCountInString(context.APIKeyRawName) > maxAPIKeyNameCharacters {
		context.APIKeyRawName = ""
	}
	if utf8.RuneCountInString(context.APIKeyName) > maxAPIKeyNameCharacters {
		context.APIKeyName = ""
		if keyID, err := strconv.ParseUint(context.APIKeyID, 10, 0); err == nil && keyID != 0 {
			context.APIKeyName = principal.MaskedAPIKeyName(context.UserID, uint(keyID))
		}
	}

	return t.sign(ctx, context)
}

// NewDelegatedToken issues a token with which the agent with agentID acts on behalf of the user with userID, from the
// delegation with delegationID, minted with the agent's key with apiKeyID. It returns the token and when it expires,
// ttl from now: DelegatedTokenTTL when ttl is not positive, and never later.
//
// The token's subject is the user and its actor (RFC 8693 "act") the agent. It carries no group, name or email. Every
// decoder but DecodeDelegatedToken refuses it, so it authenticates no one until a caller opts in to such tokens.
//
// It is never issued for a disabled or deleted agent or user, or with a key that is not the agent's or that is
// revoked or expired. Whether the delegation is valid is its caller's to check.
func (t *TokenService) NewDelegatedToken(ctx context.Context, agentID, userID uint, delegationID string, apiKeyID uint, ttl time.Duration) (string, time.Time, error) {
	delegation := DelegatedToken{AgentID: agentID, UserID: userID, DelegationID: delegationID, APIKeyID: apiKeyID}
	return t.signDelegated(ctx, delegation, ttl, TokenContext{Audience: t.serverURL})
}

// NewDelegatedLoopbackToken issues the token of a composite vMCP's loopback, and of its connections to the vMCP's
// components, for the agent that acts on behalf of the user of delegation: the delegation's agent, user, delegation
// and minting key, with the loopback's claims. Only the MCP gateway issues it, for its own connections, from a
// delegation that authenticated the request it serves (or a loopback token that did). It returns the token and when it
// expires, ttl from now: DelegatedTokenTTL when ttl is not positive, and never later.
//
// Like any token that acts on behalf of a user, every decoder but DecodeDelegatedToken refuses it, and the
// authenticator that accepts it checks the delegation, the agent, the user and the key on every use. It is never
// issued for a disabled or deleted agent or user, or with a key that is not the agent's or that is revoked or expired.
func (t *TokenService) NewDelegatedLoopbackToken(ctx context.Context, delegation DelegatedToken, loopback DelegatedLoopback, ttl time.Duration) (string, time.Time, error) {
	if err := checkDelegatedLoopback(loopback); err != nil {
		return "", time.Time{}, err
	}

	tokenContext := TokenContext{
		Audience:          loopback.Audience,
		APIKeyName:        loopback.APIKeyName,
		APIKeyRawName:     loopback.APIKeyRawName,
		UserGroups:        slices.Clone(loopback.Groups),
		MCPID:             loopback.MCPID,
		AuthorizedMCPIDs:  slices.Clone(loopback.AuthorizedMCPIDs),
		OAuthConnectionID: loopback.OAuthConnectionID,
		OAuthClientID:     loopback.OAuthClientID,
	}
	// Header-injection limits run after the loopback hop, so they cannot bound the Authorization header that carries
	// this token (NewToken does the same).
	if utf8.RuneCountInString(tokenContext.APIKeyRawName) > maxAPIKeyNameCharacters {
		tokenContext.APIKeyRawName = ""
	}
	if utf8.RuneCountInString(tokenContext.APIKeyName) > maxAPIKeyNameCharacters {
		tokenContext.APIKeyName = principal.Agent(delegation.AgentID).MaskedAPIKeyName(delegation.APIKeyID)
	}
	return t.signDelegated(ctx, delegation, ttl, tokenContext)
}

// signDelegated issues a token with which the agent of delegation acts on behalf of its user, with the claims of
// tokenContext beyond the delegation, once checkDelegation accepts delegation and ttl. It returns the token and when it
// expires.
func (t *TokenService) signDelegated(ctx context.Context, delegation DelegatedToken, ttl time.Duration, tokenContext TokenContext) (string, time.Time, error) {
	now := time.Now()
	ttl, err := t.checkDelegation(ctx, delegation, ttl, now)
	if err != nil {
		return "", time.Time{}, err
	}

	tokenContext.IssuedAt = NewTime(now.Add(-time.Second))
	tokenContext.ExpiresAt = NewTime(now.Add(ttl))
	tokenContext.UserID = principal.User(delegation.UserID).String()
	tokenContext.PrincipalKind = string(principal.KindUser)
	tokenContext.Actor = &TokenActor{Subject: principal.Agent(delegation.AgentID).String()}
	tokenContext.DelegationID = delegation.DelegationID
	tokenContext.APIKeyID = strconv.FormatUint(uint64(delegation.APIKeyID), 10)
	_, token, err := t.sign(ctx, tokenContext)
	if err != nil {
		return "", time.Time{}, err
	}
	// The token records its expiry in whole seconds.
	return token, time.Unix(tokenContext.ExpiresAt.Unix(), 0), nil
}

// checkDelegation returns an error unless a token that acts on behalf of a user may be issued from delegation at now:
// it names an agent, a user, a delegation and a minting key, both the agent and the user are active, and the key is
// the agent's and neither revoked nor expired. It returns the token's lifetime: ttl, or DelegatedTokenTTL when ttl is
// not positive. Whether the delegation itself is valid is its caller's to check.
func (t *TokenService) checkDelegation(ctx context.Context, delegation DelegatedToken, ttl time.Duration, now time.Time) (time.Duration, error) {
	switch {
	case delegation.AgentID == 0:
		return 0, errors.New("a token that acts on behalf of a user needs an agent")
	case delegation.UserID == 0:
		return 0, errors.New("a token that acts on behalf of a user needs a user")
	case delegation.DelegationID == "":
		return 0, errors.New("a token that acts on behalf of a user needs a delegation")
	case delegation.APIKeyID == 0:
		return 0, errors.New("a token that acts on behalf of a user needs the key that mints it")
	case ttl > DelegatedTokenTTL:
		return 0, fmt.Errorf("a token that acts on behalf of a user lasts at most %s", DelegatedTokenTTL)
	case ttl <= 0:
		ttl = DelegatedTokenTTL
	}
	agentID, userID, apiKeyID := delegation.AgentID, delegation.UserID, delegation.APIKeyID

	// Without the gateway database, neither the agent's status nor the user's is known, and no token is issued.
	if err := t.requireActiveAgent(ctx, agentID); err != nil {
		return 0, err
	}
	if status, err := t.gatewayClient.UserStatus(ctx, userID); err != nil {
		return 0, err
	} else if status != apitypes.UserStatusActive {
		return 0, &client.UserAccessDeniedError{
			UserID: userID,
			Status: status,
		}
	}
	key, err := t.gatewayClient.GetAgentAPIKey(ctx, agentID, apiKeyID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, fmt.Errorf("API key %d is not a key of agent %d", apiKeyID, agentID)
	} else if err != nil {
		return 0, err
	}
	if key.RevokedAt != nil {
		return 0, fmt.Errorf("API key %d of agent %d is revoked", apiKeyID, agentID)
	}
	if key.ExpiresAt != nil && !key.ExpiresAt.After(now) {
		return 0, fmt.Errorf("API key %d of agent %d is expired", apiKeyID, agentID)
	}
	return ttl, nil
}

// checkDelegatedLoopback returns an error unless loopback is what a composite vMCP's loopback token carries: an
// audience, a connection, the composite group and no group that tokens are not issued to agents with.
func checkDelegatedLoopback(loopback DelegatedLoopback) error {
	switch {
	case loopback.Audience == "":
		return errors.New("a loopback token needs an audience")
	case loopback.MCPID == "":
		return errors.New("a loopback token needs the connection it serves")
	case !slices.Contains(loopback.Groups, apitypes.GroupCompositeMCP):
		return fmt.Errorf("a loopback token needs the group %q", apitypes.GroupCompositeMCP)
	}
	if group, refused := refusedAgentTokenGroup(loopback.Groups); refused {
		return fmt.Errorf("a loopback token cannot grant group %q", group)
	}
	return nil
}

// sign issues a token with the claims of context, which its caller has checked, signed with the service's key.
func (t *TokenService) sign(ctx context.Context, context TokenContext) (*jwt.Token, string, error) {
	context.Issuer = t.serverURL
	if context.IssuedAt.IsZero() {
		context.IssuedAt = NewTime(time.Now().Add(-time.Second))
	}

	t.lock.RLock()
	privateKey := t.privateKey
	t.lock.RUnlock()

	if privateKey == nil {
		if err := t.setJWK(ctx); err != nil {
			return nil, "", err
		}

		t.lock.RLock()
		privateKey = t.privateKey
		t.lock.RUnlock()
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, context)
	s, err := token.SignedString(privateKey)
	return token, s, err
}

// checkAgentToken returns an error unless a token described by tokenContext may be issued to the agent with
// agentID: the token grants no group that tokens are not issued to agents with, and the agent is active.
func (t *TokenService) checkAgentToken(ctx context.Context, tokenContext TokenContext, agentID uint) error {
	if group, refused := refusedAgentTokenGroup(tokenContext.UserGroups); refused {
		return fmt.Errorf("a token for agent %d cannot grant group %q", agentID, group)
	}
	if principal.AdmittedActiveUser(ctx, tokenContext.UserID) {
		return nil
	}
	return t.requireActiveAgent(ctx, agentID)
}

// requireActiveAgent returns an error unless the agent with agentID is active. Unlike for a user, there is no fallback:
// without the gateway database, the agent's status is unknown, and that is an error too.
func (t *TokenService) requireActiveAgent(ctx context.Context, agentID uint) error {
	if t.gatewayClient == nil {
		return &client.AgentAccessLookupError{
			AgentID: agentID,
			Err:     errors.New("no gateway client to read the agent's status"),
		}
	}
	_, err := t.gatewayClient.ActiveAgent(ctx, agentID)
	return err
}

// refusedAgentTokenGroup returns the first of groups that no token issued to an agent carries (agentTokenGroups), and
// reports false if there is none.
func refusedAgentTokenGroup(groups []string) (string, bool) {
	i := slices.IndexFunc(groups, func(group string) bool {
		return !slices.Contains(agentTokenGroups, group)
	})
	if i < 0 {
		return "", false
	}
	return groups[i], true
}

func (t *TokenService) ServeJWKS(api api.Context) error {
	t.lock.RLock()
	jwks := t.jwks
	t.lock.RUnlock()

	if jwks == nil {
		if err := t.setJWK(api.Context()); err != nil {
			return err
		}

		t.lock.RLock()
		jwks = t.jwks
		t.lock.RUnlock()
	}

	return api.Write(jwks)
}

func (t *TokenService) replaceKey(ctx context.Context, key ed25519.PrivateKey) error {
	jwk, err := jwkset.NewJWKFromKey(key, jwkset.JWKOptions{
		Metadata: jwkset.JWKMetadataOptions{
			KID: "obot",
		},
	})
	if err != nil {
		return err
	}

	jwkSet := jwkset.NewMemoryStorage()
	if err := jwkSet.KeyWrite(ctx, jwk); err != nil {
		return err
	}

	publicJSON, err := jwkSet.JSONPublic(ctx)
	if err != nil {
		return err
	}

	t.lock.Lock()
	defer t.lock.Unlock()

	t.privateKey = key
	t.jwks = publicJSON

	return nil
}
