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
)

var (
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

	// Non-secret attribution retained across authenticated composite loopback calls.
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

// ClaimsAgentToken reports whether token is a JWT whose claims name an agent, by its subject or by its principal kind.
// The claims are read without checking the token's signature or expiry: a token is classified by what it claims to
// be, so that one that is forged, expired or signed with a replaced key is refused like any other agent token that
// fails. It is the one classifier of agent tokens, for this guard and for any code that must not treat a refused agent
// token as a user's, such as the MCP gateway's decision to withhold an interactive challenge.
func ClaimsAgentToken(token string) bool {
	claims, ok := unverifiedClaims(token)
	if !ok {
		return false
	}
	subject, _ := claims["sub"].(string)
	// The principal kind claim has the name of the user info extra that records the kind once the token authenticates.
	kind, _ := claims[principal.KindExtra].(string)
	// The subject of an agent's token is its storage encoding, "agent:<id>" (principal.Principal.String). Its prefix
	// is tested rather than the subject parsed, so that a malformed agent subject is classified as an agent's too.
	return principal.IsAgentKind(kind) || principal.ClaimsAgentOwner(subject)
}

// unverifiedClaims returns the claims of token, a JWT, without checking its signature or expiry. It is only for
// classifying a token by what it claims to be (ClaimsAgentToken), never for trusting them.
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

func (t *TokenService) DecodeToken(ctx context.Context, token string) (*TokenContext, error) {
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
		return nil, err
	}

	audiences, err := claims.GetAudience()
	if err != nil {
		return nil, err
	}

	if len(audiences) == 0 || audiences[0] == "" {
		return nil, fmt.Errorf("no audience")
	}

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

	return &TokenContext{
		IssuedAt:              NewTime(issuedAt),
		ExpiresAt:             NewTime(expiresAt),
		UserGroups:            groups,
		Audience:              getStringClaim("aud"),
		UserID:                getStringClaim("sub"),
		PrincipalKind:         getStringClaim(principal.KindExtra),
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
	}, nil
}

func (t *TokenService) NewToken(ctx context.Context, context TokenContext) (*jwt.Token, string, error) {
	if context.Audience == "" {
		return nil, "", fmt.Errorf("audience is required")
	}

	// The owner of a vMCP profile's OAuth grants stands for a profile, never for a caller, so no token is issued for
	// it, even one that would never authenticate.
	if principal.IsProfileGrant(context.UserID) {
		return nil, "", fmt.Errorf("token subject %q is a vMCP profile grant owner", context.UserID)
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
