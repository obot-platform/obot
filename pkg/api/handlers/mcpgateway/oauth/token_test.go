package oauth

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
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

func TestDoRefreshTokenPreservesScope(t *testing.T) {
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
	err = (&handler{tokenService: tokenService}).doRefreshToken(req, v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.DefaultNamespace,
			Name:      clientName,
		},
	}, refreshToken)
	require.NoError(t, err)

	var response types.OAuthToken
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	require.NotEmpty(t, response.RefreshToken)

	var refreshed v1.OAuthToken
	refreshedName := fmt.Sprintf("%x", sha256.Sum256([]byte(response.RefreshToken)))
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: refreshedName}, &refreshed))
	assert.Equal(t, "profile email", refreshed.Spec.Scope)
}
