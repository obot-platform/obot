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
	serverURL         string
}

func NewTokenService(serverURL string, gatewayClient *client.Client, credOnlyGPTClient *gptscript.GPTScript) (*TokenService, error) {
	t := &TokenService{
		gatewayClient:     gatewayClient,
		credOnlyGPTClient: credOnlyGPTClient,
		serverURL:         serverURL,
	}
	return t, nil
}

type TokenType string

const (
	TokenTypeRun TokenType = "run"
)

// EnsureJWK ensures that the JWK is created and stored in the GPTScript client. It should only be called in a controller post-start hook which only allows one to be run at a time.
func (t *TokenService) EnsureJWK(ctx context.Context) error {
	// Read the credential, if it exists, then use it.
	cred, err := t.credOnlyGPTClient.RevealCredential(ctx, []string{system.JWKCredentialContext}, system.JWKCredentialContext)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return err
	}

	var configuredKey ed25519.PrivateKey
	if keyData := cred.Env[keyEnvVar]; keyData != "" {
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
	if err := t.credOnlyGPTClient.CreateCredential(ctx, gptscript.Credential{
		Context:  system.JWKCredentialContext,
		ToolName: system.JWKCredentialContext,
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			keyEnvVar: base64.StdEncoding.EncodeToString(configuredKey),
		},
	}); err != nil {
		return err
	}

	return nil
}

// SetJWK sets the JWK in the GPTScript client. It should be called after the JWK is created and stored in the GPTScript client.
func (t *TokenService) setJWK(ctx context.Context) error {
	cred, err := t.credOnlyGPTClient.RevealCredential(ctx, []string{system.JWKCredentialContext}, system.JWKCredentialContext)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return err
	}

	value, ok := cred.Env[keyEnvVar]
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
	OAuthScope            string
	UserGroups            []string
	Picture               string
	AuthProviderName      string
	AuthProviderNamespace string
	AuthProviderUserID    string

	MCPID string

	// The following fields are for runs
	Namespace         string
	RunID             string
	ThreadID          string
	ProjectID         string
	TopLevelProjectID string
	ModelProvider     string
	Model             string
	AgentID           string
	WorkflowID        string
	WorkflowStepID    string
	Scope             string

	TokenType TokenType
}

func (t *TokenService) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return nil, false, nil
	}

	tokenContext, err := t.DecodeToken(req.Context(), token)
	if err != nil {
		return nil, false, nil
	}

	switch tokenContext.TokenType {
	case TokenTypeRun:
		return &authenticator.Response{
			User: &user.DefaultInfo{
				UID:    tokenContext.UserID,
				Name:   tokenContext.Scope,
				Groups: tokenContext.UserGroups,
				Extra: map[string][]string{
					"obot:runID":             {tokenContext.RunID},
					"obot:threadID":          {tokenContext.ThreadID},
					"obot:topLevelProjectID": {tokenContext.TopLevelProjectID},
					"obot:projectID":         {tokenContext.ProjectID},
					"obot:agentID":           {tokenContext.AgentID},
					"obot:userID":            {tokenContext.UserID},
					"obot:userName":          {tokenContext.UserName},
					"obot:userEmail":         {tokenContext.UserEmail},
				},
			},
		}, true, nil
	default:
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
					"oauthScope":              {tokenContext.OAuthScope},
				},
			},
		}, true, nil
	}
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
		OAuthScope:            getStringClaim("scope"),
		AuthProviderName:      getStringClaim("AuthProviderName"),
		AuthProviderNamespace: getStringClaim("AuthProviderNamespace"),
		AuthProviderUserID:    getStringClaim("AuthProviderUserID"),
		MCPID:                 getStringClaim("MCPID"),
		Namespace:             getStringClaim("Namespace"),
		RunID:                 getStringClaim("RunID"),
		ThreadID:              getStringClaim("ThreadID"),
		ProjectID:             getStringClaim("ProjectID"),
		TopLevelProjectID:     getStringClaim("TopLevelProjectID"),
		ModelProvider:         getStringClaim("ModelProvider"),
		Model:                 getStringClaim("Model"),
		AgentID:               getStringClaim("AgentID"),
		Scope:                 getStringClaim("Scope"),
		WorkflowID:            getStringClaim("WorkflowID"),
		WorkflowStepID:        getStringClaim("WorkflowStepID"),
		TokenType:             TokenType(getStringClaim("TokenType")),
		// These two fields were the latter names and changed the former.
		// This makes this backwards compatible with older tokens.
		UserName:  getStringClaim("name", "UserName"),
		UserEmail: getStringClaim("email", "UserEmail"),
	}, nil
}

func (t *TokenService) NewToken(ctx context.Context, context TokenContext) (string, error) {
	var picture string
	if !strings.HasPrefix(context.Picture, "data:") {
		// Don't store the picture in the token if it is a base64 encoded image
		picture = context.Picture
	}
	claims := jwt.MapClaims{
		"aud":                   context.Audience,
		"exp":                   float64(context.ExpiresAt.Unix()),
		"iat":                   float64(context.IssuedAt.Unix()),
		"sub":                   context.UserID,
		"name":                  context.UserName,
		"email":                 context.UserEmail,
		"scope":                 context.OAuthScope,
		"picture":               picture,
		"UserGroups":            strings.Join(context.UserGroups, ","),
		"AuthProviderName":      context.AuthProviderName,
		"AuthProviderNamespace": context.AuthProviderNamespace,
		"AuthProviderUserID":    context.AuthProviderUserID,
		"MCPID":                 context.MCPID,
		"Namespace":             context.Namespace,
		"RunID":                 context.RunID,
		"ThreadID":              context.ThreadID,
		"ProjectID":             context.ProjectID,
		"TopLevelProjectID":     context.TopLevelProjectID,
		"ModelProvider":         context.ModelProvider,
		"Model":                 context.Model,
		"AgentID":               context.AgentID,
		"Scope":                 context.Scope,
		"WorkflowID":            context.WorkflowID,
		"WorkflowStepID":        context.WorkflowStepID,
		"TokenType":             string(context.TokenType),
	}

	_, s, err := t.NewTokenWithClaims(ctx, claims)
	return s, err
}

func (t *TokenService) NewTokenWithClaims(ctx context.Context, claims jwt.MapClaims) (*jwt.Token, string, error) {
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

	claims["iss"] = t.serverURL
	if claims["aud"] == "" {
		claims["aud"] = t.serverURL
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	s, err := token.SignedString(privateKey)
	return token, s, err
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

const keyEnvVar = "JWK_KEY"

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
