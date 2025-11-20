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
	"strings"
	"sync"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type TokenService struct {
	lock              sync.RWMutex
	privateKey        ed25519.PrivateKey
	jwks              json.RawMessage
	gatewayClient     *client.Client
	credOnlyGPTClient *gptscript.GPTScript
	dispatcher        *dispatcher.Dispatcher
	serverURL         string
}

func NewTokenService(ctx context.Context, serverURL string, gatewayClient *client.Client, dispatcher *dispatcher.Dispatcher, credOnlyGPTClient *gptscript.GPTScript) (*TokenService, error) {
	key, err := ensureJWK(ctx, credOnlyGPTClient)
	if err != nil {
		return nil, err
	}

	t := &TokenService{
		gatewayClient:     gatewayClient,
		credOnlyGPTClient: credOnlyGPTClient,
		dispatcher:        dispatcher,
		serverURL:         serverURL,
	}

	if err = t.replaceKey(ctx, key); err != nil {
		return nil, err
	}

	return t, nil
}

func (t *TokenService) ReplaceJWK(req api.Context) error {
	// Create a key.
	_, newKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}

	if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
		Context:  system.JWKCredentialContext,
		ToolName: system.JWKCredentialContext,
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
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

type TokenContext struct {
	Audience              string
	IssuedAt              time.Time
	ExpiresAt             time.Time
	UserID                string
	UserName              string
	UserEmail             string
	UserGroups            []string
	Picture               string
	AuthProviderName      string
	AuthProviderNamespace string
	AuthProviderUserID    string

	MCPID string
}

func (t *TokenService) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return nil, false, nil
	}

	tokenContext, err := t.DecodeToken(token)
	if err != nil {
		return nil, false, nil
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			UID:    tokenContext.UserID,
			Name:   tokenContext.UserName,
			Groups: tokenContext.UserGroups,
			Extra: map[string][]string{
				"email":                   {tokenContext.UserEmail},
				"auth_provider_name":      {tokenContext.AuthProviderName},
				"auth_provider_namespace": {tokenContext.AuthProviderNamespace},
				"mcp_id":                  {tokenContext.MCPID},
				"resource":                {tokenContext.Audience},
			},
		},
	}, true, nil
}

func (t *TokenService) DecodeToken(token string) (*TokenContext, error) {
	tk, err := jwt.Parse(token, func(*jwt.Token) (any, error) {
		t.lock.RLock()
		defer t.lock.RUnlock()
		return t.privateKey.Public(), nil
	}, jwt.WithIssuer(t.serverURL))
	if err != nil {
		return nil, err
	}
	claims, ok := tk.Claims.(jwt.MapClaims)
	if !ok {
		return nil, err
	}

	var groups []string
	if userGroups, ok := claims["UserGroups"].(string); ok {
		groups = strings.Split(userGroups, ",")
		groups = slices.DeleteFunc(groups, func(s string) bool { return s == "" })
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
		IssuedAt:              issuedAt,
		ExpiresAt:             expiresAt,
		UserGroups:            groups,
		Audience:              getStringClaim("aud"),
		UserID:                getStringClaim("sub"),
		Picture:               getStringClaim("picture"),
		AuthProviderName:      getStringClaim("AuthProviderName"),
		AuthProviderNamespace: getStringClaim("AuthProviderNamespace"),
		AuthProviderUserID:    getStringClaim("AuthProviderUserID"),
		MCPID:                 getStringClaim("MCPID"),
		// These two fields were the latter names and changed the former.
		// This makes this backwards compatible with older tokens.
		UserName:  getStringClaim("name", "UserName"),
		UserEmail: getStringClaim("email", "UserEmail"),
	}, nil
}

func (t *TokenService) NewToken(context TokenContext) (string, error) {
	claims := jwt.MapClaims{
		"aud":                   context.Audience,
		"exp":                   float64(context.ExpiresAt.Unix()),
		"iat":                   float64(context.IssuedAt.Unix()),
		"sub":                   context.UserID,
		"name":                  context.UserName,
		"email":                 context.UserEmail,
		"picture":               context.Picture,
		"UserGroups":            strings.Join(context.UserGroups, ","),
		"AuthProviderName":      context.AuthProviderName,
		"AuthProviderNamespace": context.AuthProviderNamespace,
		"AuthProviderUserID":    context.AuthProviderUserID,
		"MCPID":                 context.MCPID,
	}

	_, s, err := t.NewTokenWithClaims(claims)
	return s, err
}

func (t *TokenService) NewTokenWithClaims(claims jwt.MapClaims) (*jwt.Token, string, error) {
	claims["iss"] = t.serverURL
	if claims["aud"] == "" {
		claims["aud"] = t.serverURL
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	s, err := token.SignedString(t.privateKey)
	return token, s, err
}

func (t *TokenService) ServeJWKS(api api.Context) error {
	return api.Write(t.JWKS())
}

func (t *TokenService) JWKS() []byte {
	t.lock.RLock()
	defer t.lock.RUnlock()
	return t.jwks
}

func (t *TokenService) EncodedJWKS() string {
	return base64.StdEncoding.EncodeToString(t.JWKS())
}

const keyEnvVar = "JWK_KEY"

func ensureJWK(ctx context.Context, gptClient *gptscript.GPTScript) (ed25519.PrivateKey, error) {
	// Read the credential, if it exists, then use it.
	cred, err := gptClient.RevealCredential(ctx, []string{system.JWKCredentialContext}, system.JWKCredentialContext)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return nil, err
	}

	var configuredKey ed25519.PrivateKey
	if keyData := cred.Env[keyEnvVar]; keyData != "" {
		configuredKey, err = base64.StdEncoding.DecodeString(keyData)
		if err != nil {
			return nil, err
		}
	} else {
		// Create a key.
		_, configuredKey, err = ed25519.GenerateKey(nil)
		if err != nil {
			return nil, err
		}
	}

	// Write the key to the JWK Set storage.
	if err := gptClient.CreateCredential(ctx, gptscript.Credential{
		Context:  system.JWKCredentialContext,
		ToolName: system.JWKCredentialContext,
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			keyEnvVar: base64.StdEncoding.EncodeToString(configuredKey),
		},
	}); err != nil {
		return nil, err
	}

	return configuredKey, nil
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
