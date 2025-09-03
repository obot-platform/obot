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
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type TokenService struct {
	lock          sync.RWMutex
	privateKey    ed25519.PrivateKey
	jwks          json.RawMessage
	gatewayClient *client.Client
	dispatcher    *dispatcher.Dispatcher
	serverURL     string
}

func NewTokenService(ctx context.Context, serverURL string, gatewayClient *client.Client, dispatcher *dispatcher.Dispatcher, gptClient *gptscript.GPTScript) (*TokenService, error) {
	key, err := ensureJWK(ctx, gptClient)
	if err != nil {
		return nil, err
	}

	t := &TokenService{
		gatewayClient: gatewayClient,
		dispatcher:    dispatcher,
		serverURL:     serverURL,
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
	IssuedAt              time.Time
	NotBefore             time.Time
	ExpiresAt             time.Time
	UserID                string
	UserName              string
	UserEmail             string
	UserGroups            []string
	AuthProviderName      string
	AuthProviderNamespace string
	HashedSessionID       string
}

func (t *TokenService) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	token := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return nil, false, nil
	}

	tokenContext, err := t.decodeToken(token)
	if err != nil {
		return nil, false, nil
	}

	if tokenContext.HashedSessionID != "" {
		if err = server.HandleHashedSessionID(req, t.gatewayClient, t.dispatcher, tokenContext.HashedSessionID, tokenContext.AuthProviderNamespace, tokenContext.AuthProviderName); err != nil {
			return nil, false, err
		}
	}

	groups := append([]string{authz.AuthenticatedGroup}, tokenContext.UserGroups...)
	return &authenticator.Response{
		User: &user.DefaultInfo{
			UID:    tokenContext.UserID,
			Name:   tokenContext.UserName,
			Groups: groups,
			Extra: map[string][]string{
				"email":                   {tokenContext.UserEmail},
				"auth_provider_name":      {tokenContext.AuthProviderName},
				"auth_provider_namespace": {tokenContext.AuthProviderNamespace},
			},
		},
	}, true, nil
}

func (t *TokenService) decodeToken(token string) (*TokenContext, error) {
	tk, err := jwt.Parse(token, func(*jwt.Token) (any, error) {
		t.lock.RLock()
		defer t.lock.RUnlock()
		return t.privateKey.Public(), nil
	}, jwt.WithIssuer(t.serverURL), jwt.WithAudience(t.serverURL))
	if err != nil {
		return nil, err
	}
	claims, ok := tk.Claims.(jwt.MapClaims)
	if !ok {
		return nil, err
	}

	groups := strings.Split(claims["UserGroups"].(string), ",")
	groups = slices.DeleteFunc(groups, func(s string) bool { return s == "" })

	return &TokenContext{
		UserID:                claims["UserID"].(string),
		UserName:              claims["UserName"].(string),
		UserEmail:             claims["UserEmail"].(string),
		UserGroups:            groups,
		AuthProviderName:      claims["AuthProviderName"].(string),
		AuthProviderNamespace: claims["AuthProviderNamespace"].(string),
		HashedSessionID:       claims["HashedSessionID"].(string),
	}, nil
}

func (t *TokenService) NewToken(context TokenContext) (string, error) {
	claims := jwt.MapClaims{
		"iss":                   t.serverURL,
		"aud":                   t.serverURL,
		"exp":                   context.ExpiresAt.Unix(),
		"nbf":                   context.NotBefore.Unix(),
		"iat":                   context.IssuedAt.Unix(),
		"sub":                   context.UserID,
		"UserID":                context.UserID,
		"UserName":              context.UserName,
		"UserEmail":             context.UserEmail,
		"UserGroups":            strings.Join(context.UserGroups, ","),
		"AuthProviderName":      context.AuthProviderName,
		"AuthProviderNamespace": context.AuthProviderNamespace,
		"HashedSessionID":       context.HashedSessionID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	return token.SignedString(t.privateKey)
}

func (t *TokenService) ServeJWKS(api api.Context) error {
	t.lock.RLock()
	jwks := t.jwks
	t.lock.RUnlock()

	return api.Write(jwks)
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
