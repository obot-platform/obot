package oauth

import (
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
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDoRefreshTokenRotatesTokenAndPreservesScope(t *testing.T) {
	const (
		baseURL      = "https://obot.example.com"
		clientName   = "oauth-client"
		mcpID        = system.SystemMCPServerPrefix + "test"
		refreshToken = "old-refresh-token"
	)

	storage := clientfake.NewClientBuilder().
		WithScheme(scheme.Scheme).
		WithObjects(&v1.SystemMCPServer{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.DefaultNamespace,
				Name:      mcpID,
			},
		}).
		Build()

	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())

	gatewayClient := gatewayclient.New(t.Context(), db, storage, nil, nil, nil, nil, time.Hour, 10, 90, 90, true)
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

	tokenName := fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken)))
	storageToken := &v1.OAuthToken{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.DefaultNamespace,
			Name:      tokenName,
		},
		Spec: v1.OAuthTokenSpec{
			ClientID: clientName,
			Resource: baseURL,
			Scope:    "profile email",
			UserID:   42,
			MCPID:    mcpID,
		},
	}
	require.NoError(t, storage.Create(t.Context(), storageToken))

	tokenService, err := persistent.NewTokenService(baseURL, gatewayClient)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	req := api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest("POST", "/oauth/token", nil),
		Storage:        storage,
		GatewayClient:  gatewayClient,
	}
	oauthClient := v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.DefaultNamespace,
			Name:      clientName,
		},
	}
	h := &handler{tokenService: tokenService}
	err = h.doRefreshToken(req, oauthClient, refreshToken)
	require.NoError(t, err)

	var response types.OAuthToken
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	require.NotEmpty(t, response.RefreshToken)

	var refreshed v1.OAuthToken
	refreshedName := fmt.Sprintf("%x", sha256.Sum256([]byte(response.RefreshToken)))
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: refreshedName}, &refreshed))
	assert.Equal(t, "profile email", refreshed.Spec.Scope)

	err = h.doRefreshToken(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest("POST", "/oauth/token", nil),
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
}

func TestMCPTokenGroupsForVersionedResource(t *testing.T) {
	const (
		baseURL = "https://obot.example.com"
		entryID = "catalog-entry"
	)
	entry := &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: entryID, Namespace: system.DefaultNamespace},
	}
	version := &v1.MCPServerCatalogEntryVersion{
		ObjectMeta: metav1.ObjectMeta{Name: v1.MCPServerCatalogEntryVersionName(entryID, 2), Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerCatalogEntryVersionSpec{
			MCPServerCatalogEntryName: entryID,
			Version:                   2,
			Active:                    true,
		},
	}
	storage := clientfake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(entry, version).Build()
	h := &handler{baseURL: baseURL}
	resource := baseURL + "/versioned-mcp-connect/" + entryID + "/2"

	groups, err := h.mcpTokenGroups(t.Context(), storage, resource, true)
	require.NoError(t, err)
	assert.Equal(t, []string{types.GroupVersionedMCP, types.GroupAuthenticated}, groups)
	assert.NotContains(t, groups, types.GroupAdmin)
	assert.NotContains(t, groups, types.GroupMCP)

	_, err = h.mcpTokenGroups(t.Context(), storage, resource, false)
	require.ErrorContains(t, err, "administrator access is required")
	_, err = h.mcpTokenGroups(t.Context(), storage, resource+"/messages", true)
	require.ErrorContains(t, err, "invalid versioned MCP resource")

	version.Spec.Active = false
	require.NoError(t, storage.Update(t.Context(), version))
	_, err = h.mcpTokenGroups(t.Context(), storage, resource, true)
	require.ErrorContains(t, err, "active version 2")
}

func TestVersionedTokenEndpointRejectsAuthorizationCodeForAnotherVersion(t *testing.T) {
	const code = "authorization-code"
	hashedCode := fmt.Sprintf("%x", sha256.Sum256([]byte(code)))
	authRequest := &v1.OAuthAuthRequest{
		ObjectMeta: metav1.ObjectMeta{Name: "auth-request", Namespace: system.DefaultNamespace},
		Spec: v1.OAuthAuthRequestSpec{
			ClientID:       "client",
			HashedAuthCode: hashedCode,
			Resource:       "https://obot.example.com/versioned-mcp-connect/entry/1",
		},
	}
	storage := clientfake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(authRequest).
		WithIndex(&v1.OAuthAuthRequest{}, "spec.hashedAuthCode", func(obj kclient.Object) []string {
			return []string{obj.(*v1.OAuthAuthRequest).Spec.HashedAuthCode}
		}).Build()
	request := httptest.NewRequest("POST", "/oauth/token/versioned-mcp-connect/entry/2", nil)
	request.SetPathValue("entry_id", "entry")
	request.SetPathValue("version", "2")
	req := api.Context{Request: request, ResponseWriter: httptest.NewRecorder(), Storage: storage}

	err := (&handler{baseURL: "https://obot.example.com"}).doAuthorizationCode(req, v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{Name: "client", Namespace: system.DefaultNamespace},
	}, code, "")
	require.ErrorContains(t, err, "does not belong")
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(authRequest), &v1.OAuthAuthRequest{}))
}

func TestVersionedTokenEndpointRejectsRefreshTokenForAnotherEntry(t *testing.T) {
	const refreshToken = "refresh-token"
	token := &v1.OAuthToken{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken))), Namespace: system.DefaultNamespace,
		},
		Spec: v1.OAuthTokenSpec{
			ClientID: "client",
			Resource: "https://obot.example.com/versioned-mcp-connect/entry-a/2",
		},
	}
	storage := clientfake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(token).Build()
	request := httptest.NewRequest("POST", "/oauth/token/versioned-mcp-connect/entry-b/2", nil)
	request.SetPathValue("entry_id", "entry-b")
	request.SetPathValue("version", "2")
	req := api.Context{Request: request, ResponseWriter: httptest.NewRecorder(), Storage: storage}

	err := (&handler{baseURL: "https://obot.example.com"}).doRefreshToken(req, v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{Name: "client", Namespace: system.DefaultNamespace},
	}, refreshToken)
	require.ErrorContains(t, err, "does not belong")
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(token), &v1.OAuthToken{}))
}

func TestGenericTokenEndpointRejectsVersionedRefreshToken(t *testing.T) {
	const refreshToken = "refresh-token"
	token := &v1.OAuthToken{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken))), Namespace: system.DefaultNamespace,
		},
		Spec: v1.OAuthTokenSpec{
			ClientID: "client",
			Resource: "https://obot.example.com/versioned-mcp-connect/entry/2",
		},
	}
	storage := clientfake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(token).Build()
	req := api.Context{
		Request: httptest.NewRequest("POST", "/oauth/token", nil), ResponseWriter: httptest.NewRecorder(), Storage: storage,
	}

	err := (&handler{baseURL: "https://obot.example.com"}).doRefreshToken(req, v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{Name: "client", Namespace: system.DefaultNamespace},
	}, refreshToken)
	require.ErrorContains(t, err, "exact token endpoint")
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(token), &v1.OAuthToken{}))
}
