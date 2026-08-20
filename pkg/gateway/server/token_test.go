package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestTokenRequestCreatesDeviceRequest(t *testing.T) {
	s, gatewayClient := newTokenRequestTestServer(t)
	body := map[string]any{
		"id":                "caller-supplied-id",
		"name":              "CLI token",
		"description":       "created by server test",
		"providerName":      "github",
		"providerNamespace": "default",
		"noExpiration":      true,
		"scopes": map[string]any{
			"canAccessAPI":         true,
			"canAccessDeviceScans": true,
		},
	}
	ctx, recorder := newTokenRequestAPIContext(t, gatewayClient, http.MethodPost, "/api/token-request", body, nil)
	if err := s.tokenRequest(ctx); err != nil {
		t.Fatal(err)
	}

	var response tokenRequestResponse
	decodeTokenRequestResponse(t, recorder, &response)
	if response.ID == "" || response.ID == "caller-supplied-id" {
		t.Fatalf("response ID = %q, want a server-generated ID", response.ID)
	}
	if response.DeviceCode == "" {
		t.Fatal("expected a generated device code")
	}

	verificationURL, err := url.Parse(response.TokenPath)
	if err != nil {
		t.Fatal(err)
	}
	if verificationURL.Path != "/oauth2/start" {
		t.Fatalf("verification path = %q, want /oauth2/start", verificationURL.Path)
	}
	if got := verificationURL.Query().Get("rd"); got != "/auth/device-code" {
		t.Fatalf("rd = %q, want /auth/device-code", got)
	}
	if got := verificationURL.Query().Get("obot-auth-provider"); got != "default/github" {
		t.Fatalf("provider = %q, want default/github", got)
	}
	if strings.Contains(response.TokenPath, response.ID) || strings.Contains(response.TokenPath, response.DeviceCode) {
		t.Fatalf("verification URL leaks request-bound data: %s", response.TokenPath)
	}

	var stored types.TokenRequest
	if err := s.db.WithContext(t.Context()).First(&stored, "id = ?", response.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Purpose != types.TokenRequestPurposeDeviceLogin || stored.Name != "CLI token" || !stored.NoExpiration {
		t.Fatalf("stored token request = %+v, want device purpose and copied metadata", stored)
	}
	if !stored.Scopes.CanAccessAPI || !stored.Scopes.CanAccessDeviceScans {
		t.Fatalf("stored scopes = %+v, want requested scopes", stored.Scopes)
	}
}

func TestTokenRequestValidatesProvider(t *testing.T) {
	s, gatewayClient := newTokenRequestTestServer(t)
	tests := []struct {
		name string
		body map[string]any
	}{
		{
			name: "missing provider namespace",
			body: map[string]any{"providerName": "github"},
		},
		{
			name: "unconfigured provider",
			body: map[string]any{"providerName": "gitlab", "providerNamespace": "default"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, _ := newTokenRequestAPIContext(t, gatewayClient, http.MethodPost, "/api/token-request", tt.body, nil)
			assertHTTPError(t, s.tokenRequest(ctx), http.StatusBadRequest, "")
		})
	}
}

func TestDeviceCodeVerificationUsesAuthenticatedUser(t *testing.T) {
	s, gatewayClient := newTokenRequestTestServer(t)
	request := &types.TokenRequest{
		Name:        "CLI token",
		Description: "browser verified",
		Scopes:      types.APIKeyScopes{CanAccessAPI: true},
	}
	code, err := gatewayClient.CreateDeviceTokenRequest(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}

	body := map[string]any{"code": code, "userId": 999}
	userInfo := &user.DefaultInfo{UID: "42", Groups: []string{clienttypes.GroupAPI, clienttypes.GroupAuthenticated}}
	ctx, recorder := newTokenRequestAPIContext(t, gatewayClient, http.MethodPost, "/api/token-request/verify", body, userInfo)
	if err := s.verifyDeviceCode(ctx); err != nil {
		t.Fatal(err)
	}
	var response map[string]bool
	decodeTokenRequestResponse(t, recorder, &response)
	if !response["authorized"] {
		t.Fatalf("verification response = %+v, want authorized", response)
	}

	keys, err := gatewayClient.ListAPIKeys(t.Context(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 || keys[0].UserID != 42 {
		t.Fatalf("authenticated user's keys = %+v, want one key for user 42", keys)
	}
	otherKeys, err := gatewayClient.ListAPIKeys(t.Context(), 999)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherKeys) != 0 {
		t.Fatalf("body-supplied user's keys = %+v, want none", otherKeys)
	}
}

func TestDeviceCodeVerificationReturnsGenericInvalidError(t *testing.T) {
	s, gatewayClient := newTokenRequestTestServer(t)
	request := new(types.TokenRequest)
	code, err := gatewayClient.CreateDeviceTokenRequest(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.WithContext(t.Context()).Model(request).Update("request_expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}

	userInfo := &user.DefaultInfo{UID: "42", Groups: []string{clienttypes.GroupAPI}}
	for _, candidate := range []string{"2345-6789", code} {
		ctx, _ := newTokenRequestAPIContext(t, gatewayClient, http.MethodPost, "/api/token-request/verify", map[string]string{"code": candidate}, userInfo)
		assertHTTPError(t, s.verifyDeviceCode(ctx), http.StatusBadRequest, client.ErrInvalidOrExpiredDeviceCode.Error())
	}
}

func TestTokenRequestPollingPendingSuccessAndExpired(t *testing.T) {
	s, gatewayClient := newTokenRequestTestServer(t)

	pending := new(types.TokenRequest)
	if _, err := gatewayClient.CreateDeviceTokenRequest(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	pendingResponse := pollTokenRequest(t, s, gatewayClient, pending.ID)
	if pendingResponse.Token != "" {
		t.Fatalf("pending token = %q, want empty", pendingResponse.Token)
	}

	expired := new(types.TokenRequest)
	if _, err := gatewayClient.CreateDeviceTokenRequest(t.Context(), expired); err != nil {
		t.Fatal(err)
	}
	if err := s.db.WithContext(t.Context()).Model(expired).Update("request_expires_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	expiredContext, expiredRecorder := newTokenRequestAPIContext(t, gatewayClient, http.MethodGet, "/api/token-request/"+expired.ID, nil, nil)
	expiredContext.SetPathValue("id", expired.ID)
	if err := s.checkForToken(expiredContext); err != nil {
		t.Fatal(err)
	}
	var expiredResponse map[string]string
	decodeTokenRequestResponse(t, expiredRecorder, &expiredResponse)
	if expiredResponse["error"] != "token request expired" {
		t.Fatalf("expired response = %+v, want token request expired", expiredResponse)
	}

	success := new(types.TokenRequest)
	code, err := gatewayClient.CreateDeviceTokenRequest(t.Context(), success)
	if err != nil {
		t.Fatal(err)
	}
	if err := gatewayClient.AuthorizeTokenRequestByDeviceCode(t.Context(), 7, code); err != nil {
		t.Fatal(err)
	}
	successResponse := pollTokenRequest(t, s, gatewayClient, success.ID)
	if !strings.HasPrefix(successResponse.Token, "ok1-7-") {
		t.Fatalf("success token = %q, want key for user 7", successResponse.Token)
	}
	var stored types.TokenRequest
	if err := s.db.WithContext(t.Context()).First(&stored, "id = ?", success.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.TokenRetrieved {
		t.Fatal("expected successful poll to mark token retrieved")
	}
}

func TestTokenRequestOAuthIsSetupOnly(t *testing.T) {
	s, gatewayClient := newTokenRequestTestServer(t)
	device := new(types.TokenRequest)
	if _, err := gatewayClient.CreateDeviceTokenRequest(t.Context(), device); err != nil {
		t.Fatal(err)
	}
	if _, err := gatewayClient.CreateTokenRequestState(t.Context(), device.ID); err == nil {
		t.Fatal("expected device request to be rejected by request-bound OAuth state creation")
	}

	setup := &types.TokenRequest{
		ID:                    "setup-request",
		Purpose:               types.TokenRequestPurposeSetup,
		RequestExpiresAt:      time.Now().Add(15 * time.Minute),
		CompletionRedirectURL: "/setup/oauth-complete",
	}
	if err := gatewayClient.CreateTokenRequest(t.Context(), setup); err != nil {
		t.Fatal(err)
	}
	state, err := gatewayClient.CreateTokenRequestState(t.Context(), setup.ID)
	if err != nil {
		t.Fatal(err)
	}
	verified, err := gatewayClient.VerifyTokenRequestState(t.Context(), state)
	if err != nil {
		t.Fatal(err)
	}
	if verified.ID != setup.ID || verified.Purpose != types.TokenRequestPurposeSetup {
		t.Fatalf("verified request = %+v, want setup request", verified)
	}

	deviceRedirectContext, _ := newTokenRequestAPIContext(t, gatewayClient, http.MethodGet, "/api/token-request/"+device.ID+"/default/github", nil, nil)
	deviceRedirectContext.SetPathValue("id", device.ID)
	deviceRedirectContext.SetPathValue("namespace", "default")
	deviceRedirectContext.SetPathValue("name", "github")
	assertHTTPError(t, s.redirectForTokenRequest(deviceRedirectContext), http.StatusNotFound, "token not found")
}

func pollTokenRequest(t *testing.T, s *Server, gatewayClient *client.Client, id string) refreshTokenResponse {
	t.Helper()
	ctx, recorder := newTokenRequestAPIContext(t, gatewayClient, http.MethodGet, "/api/token-request/"+id, nil, nil)
	ctx.SetPathValue("id", id)
	if err := s.checkForToken(ctx); err != nil {
		t.Fatal(err)
	}
	var response refreshTokenResponse
	decodeTokenRequestResponse(t, recorder, &response)
	return response
}

func newTokenRequestTestServer(t *testing.T) (*Server, *client.Client) {
	t.Helper()
	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("create storage services: %v", err)
	}
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("create gateway database: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("migrate gateway database: %v", err)
	}

	provider := &v1.AuthProvider{
		Name: "github", Namespace: system.DefaultNamespace,
		Status: v1.AuthProviderStatus{Configured: true},
	}
	defaultRole := &v1.UserDefaultRoleSetting{
		Name: system.DefaultRoleSettingName, Namespace: system.DefaultNamespace,
		Spec: v1.UserDefaultRoleSettingSpec{Role: clienttypes.RoleBasic},
	}
	storageClient := clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(provider, defaultRole).
		WithIndex(&v1.AuthProvider{}, "status.configured", func(obj kclient.Object) []string {
			return []string{strconv.FormatBool(obj.(*v1.AuthProvider).Status.Configured)}
		}).
		Build()
	gatewayClient := client.New(t.Context(), db, storageClient, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, false)
	t.Cleanup(func() { _ = gatewayClient.Close() })
	if err := gatewayClient.UpsertCredential(t.Context(), types.Credential{
		Context: provider.Name,
		Name:    provider.Name,
		Secrets: map[string]string{},
	}); err != nil {
		t.Fatalf("create provider credential: %v", err)
	}

	providerDispatcher := dispatcher.New(nil, storageClient, gatewayClient, nil, "", "", "")
	return &Server{
		db:         db,
		baseURL:    "https://obot.example.com",
		dispatcher: providerDispatcher,
	}, gatewayClient
}

func newTokenRequestAPIContext(t *testing.T, gatewayClient *client.Client, method, path string, body any, userInfo user.Info) (api.Context, *httptest.ResponseRecorder) {
	t.Helper()
	var requestBody bytes.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		requestBody = *bytes.NewReader(data)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, &requestBody)
	return api.Context{
		ResponseWriter: recorder,
		Request:        request,
		GatewayClient:  gatewayClient,
		User:           userInfo,
	}, recorder
}

func decodeTokenRequestResponse(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response: %v (body %s)", err, recorder.Body.String())
	}
}

func assertHTTPError(t *testing.T, err error, code int, message string) {
	t.Helper()
	var httpErr *clienttypes.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %v, want HTTP error", err)
	}
	if httpErr.Code != code {
		t.Fatalf("HTTP error code = %d, want %d", httpErr.Code, code)
	}
	if message != "" && httpErr.Message != message {
		t.Fatalf("HTTP error message = %q, want %q", httpErr.Message, message)
	}
}
