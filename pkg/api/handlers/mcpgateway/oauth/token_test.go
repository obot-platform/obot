package oauth

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type consumeOAuthTokenAfterGetStorage struct {
	storage.Client
}

func (s *consumeOAuthTokenAfterGetStorage) Get(ctx context.Context, key kclient.ObjectKey, obj kclient.Object, opts ...kclient.GetOption) error {
	if err := s.Client.Get(ctx, key, obj, opts...); err != nil {
		return err
	}
	if _, ok := obj.(*v1.OAuthToken); ok {
		return s.Delete(ctx, obj)
	}
	return nil
}

func newOAuthTokenTestServices(t *testing.T, objects ...kclient.Object) (storage.Client, *gatewayclient.Client, *persistent.TokenService) {
	t.Helper()

	objects = append(objects, &v1.SystemMCPServer{
		Namespace: system.DefaultNamespace,
		Name:      system.SystemMCPServerPrefix + "test",
	})
	storage := clientfake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithIndex(&v1.VMCPInstance{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.LegacySlug} }).
		WithIndex(&v1.OAuthAuthRequest{}, "spec.hashedAuthCode", func(obj kclient.Object) []string { return []string{obj.(*v1.OAuthAuthRequest).Spec.HashedAuthCode} }).
		WithObjects(objects...).
		Build()

	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())

	gatewayClient := gatewayclient.New(t.Context(), db, storage, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, gatewayClient.Close()) })
	require.NoError(t, db.WithContext(t.Context()).Create(&gatewaytypes.User{
		ID:       42,
		Username: "alice",
		Email:    "alice@example.com",
		Role:     types.RoleBasic,
	}).Error)

	_, privateKey, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.JWKCredentialContext,
		Name:    system.JWKCredentialContext,
		Secrets: map[string]string{
			"JWK_KEY": base64.StdEncoding.EncodeToString(privateKey),
		},
	}))
	tokenService, err := persistent.NewTokenService("https://obot.example.com", gatewayClient)
	require.NoError(t, err)

	return storage, gatewayClient, tokenService
}

func TestDoAuthorizationCodeScopesTokenToAudience(t *testing.T) {
	const (
		baseURL    = "https://obot.example.com"
		clientName = "oauth-client"
		mcpID      = system.SystemMCPServerPrefix + "test"
		code       = "authorization-code"
	)

	tests := []struct {
		name           string
		audience       string
		wantAuthorized persistent.StringSlice
	}{
		{
			name:           "distinct audience",
			audience:       "multi-user-server",
			wantAuthorized: persistent.StringSlice{"multi-user-server"},
		},
		{
			name: "empty audience",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authRequest := &v1.OAuthAuthRequest{
				Namespace: system.DefaultNamespace,
				Name:      "oauth-request",
				Spec: v1.OAuthAuthRequestSpec{
					ClientID:       clientName,
					Resource:       baseURL + "/mcp-connect/" + tt.audience,
					Scope:          "profile email",
					HashedAuthCode: fmt.Sprintf("%x", sha256.Sum256([]byte(code))),
					UserID:         42,
					MCPID:          mcpID,
					Audience:       tt.audience,
				},
			}
			storage, gatewayClient, tokenService := newOAuthTokenTestServices(t, authRequest)
			recorder := httptest.NewRecorder()
			h := &handler{tokenService: tokenService}
			err := h.doAuthorizationCode(api.Context{
				ResponseWriter: recorder,
				Request:        httptest.NewRequest(http.MethodPost, "/oauth/token", nil),
				Storage:        storage,
				GatewayClient:  gatewayClient,
			}, v1.OAuthClient{Namespace: system.DefaultNamespace, Name: clientName}, code, "")
			require.NoError(t, err)

			var response types.OAuthToken
			require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
			claims, err := tokenService.DecodeToken(t.Context(), response.AccessToken)
			require.NoError(t, err)
			assert.Equal(t, mcpID, claims.MCPID)
			assert.Equal(t, tt.wantAuthorized, claims.AuthorizedMCPIDs)

			var refreshToken v1.OAuthToken
			require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{
				Namespace: system.DefaultNamespace,
				Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(response.RefreshToken))),
			}, &refreshToken))
			assert.Equal(t, tt.audience, refreshToken.Spec.Audience)
		})
	}
}

func TestDoRefreshTokenRotatesTokenAndPreservesScope(t *testing.T) {
	const (
		baseURL      = "https://obot.example.com"
		clientName   = "oauth-client"
		mcpID        = system.SystemMCPServerPrefix + "test"
		audience     = "multi-user-server"
		refreshToken = "old-refresh-token"
	)

	tokenName := fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken)))
	storageToken := &v1.OAuthToken{
		Namespace: system.DefaultNamespace,
		Name:      tokenName,
		Spec: v1.OAuthTokenSpec{
			ClientID: clientName,
			Resource: baseURL + "/mcp-connect/" + audience,
			Scope:    "profile email",
			UserID:   42,
			MCPID:    mcpID,
			Audience: audience,
		},
	}
	storage, gatewayClient, tokenService := newOAuthTokenTestServices(t, storageToken)
	recorder := httptest.NewRecorder()
	req := api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodPost, "/oauth/token", nil),
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}
	oauthClient := v1.OAuthClient{
		Namespace: system.DefaultNamespace,
		Name:      clientName,
	}
	h := &handler{baseURL: baseURL, tokenService: tokenService}
	err := h.doRefreshToken(req, oauthClient, refreshToken)
	require.NoError(t, err)

	var response types.OAuthToken
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	require.NotEmpty(t, response.RefreshToken)
	claims, err := tokenService.DecodeToken(t.Context(), response.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, mcpID, claims.MCPID)
	assert.Equal(t, persistent.StringSlice{audience}, claims.AuthorizedMCPIDs)

	var refreshed v1.OAuthToken
	refreshedName := fmt.Sprintf("%x", sha256.Sum256([]byte(response.RefreshToken)))
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: refreshedName}, &refreshed))
	assert.Equal(t, "profile email", refreshed.Spec.Scope)
	assert.Equal(t, audience, refreshed.Spec.Audience)

	emptyAudienceRefreshToken := "empty-audience-refresh-token"
	require.NoError(t, storage.Create(t.Context(), &v1.OAuthToken{
		Namespace: system.DefaultNamespace,
		Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(emptyAudienceRefreshToken))),
		Spec: v1.OAuthTokenSpec{
			ClientID: clientName,
			Resource: baseURL + "/mcp-connect/" + audience,
			UserID:   42,
			MCPID:    mcpID,
		},
	}))
	emptyAudienceRecorder := httptest.NewRecorder()
	err = h.doRefreshToken(api.Context{
		ResponseWriter: emptyAudienceRecorder,
		Request:        httptest.NewRequest(http.MethodPost, "/oauth/token", nil),
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}, oauthClient, emptyAudienceRefreshToken)
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(emptyAudienceRecorder.Body).Decode(&response))
	claims, err = tokenService.DecodeToken(t.Context(), response.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, persistent.StringSlice{audience}, claims.AuthorizedMCPIDs)

	var legacyRefreshed v1.OAuthToken
	legacyRefreshedName := fmt.Sprintf("%x", sha256.Sum256([]byte(response.RefreshToken)))
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: legacyRefreshedName}, &legacyRefreshed))
	assert.Equal(t, audience, legacyRefreshed.Spec.Audience)

	err = h.doRefreshToken(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/oauth/token", nil),
		Storage:        storage,
	}, oauthClient, refreshToken)
	require.Error(t, err)

	var errHTTP *types.ErrHTTP
	require.ErrorAs(t, err, &errHTTP)
	assert.Equal(t, http.StatusBadRequest, errHTTP.Code)

	var oauthErr oauthError
	require.NoError(t, json.Unmarshal([]byte(errHTTP.Message), &oauthErr))
	assert.Equal(t, "invalid_grant", string(oauthErr.Code))
	assert.Equal(t, "Obot: refresh_token is invalid", oauthErr.Description)

	staleRefreshToken := "deleted-server-refresh-token"
	require.NoError(t, storage.Create(t.Context(), &v1.OAuthToken{
		Namespace: system.DefaultNamespace,
		Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(staleRefreshToken))),
		Spec: v1.OAuthTokenSpec{
			ClientID: clientName,
			Resource: baseURL,
			UserID:   42,
			MCPID:    system.MCPServerPrefix + "deleted",
		},
	}))

	err = h.doRefreshToken(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/oauth/token", nil),
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}, oauthClient, staleRefreshToken)
	require.Error(t, err)
	require.ErrorAs(t, err, &errHTTP)
	require.NoError(t, json.Unmarshal([]byte(errHTTP.Message), &oauthErr))
	assert.Equal(t, "invalid_grant", string(oauthErr.Code))
	assert.Equal(t, "Obot: invalid MCP server", oauthErr.Description)
	err = storage.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(staleRefreshToken))),
	}, &v1.OAuthToken{})
	require.True(t, apierrors.IsNotFound(err), "expected NotFound after consuming stale refresh token, got %v", err)

	racedRefreshToken := "concurrently-consumed-refresh-token"
	require.NoError(t, storage.Create(t.Context(), &v1.OAuthToken{
		Namespace: system.DefaultNamespace,
		Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(racedRefreshToken))),
		Spec: v1.OAuthTokenSpec{
			ClientID: clientName,
			Resource: baseURL,
			UserID:   42,
			MCPID:    system.MCPServerPrefix + "deleted",
		},
	}))

	err = h.doRefreshToken(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/oauth/token", nil),
		Storage:        &consumeOAuthTokenAfterGetStorage{Client: storage},
		GatewayClient:  gatewayClient,
	}, oauthClient, racedRefreshToken)
	require.Error(t, err)
	require.ErrorAs(t, err, &errHTTP)
	require.NoError(t, json.Unmarshal([]byte(errHTTP.Message), &oauthErr))
	assert.Equal(t, "invalid_grant", string(oauthErr.Code))
	assert.Equal(t, "Obot: refresh_token is invalid", oauthErr.Description)
}
