package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/mcpgateway"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestStaticOAuthCredentialTestStartsWithRealMetadataAndReturnsSafeStatus(t *testing.T) {
	provider := newStaticOAuthTestProvider(t)
	gateway := newOAuthCredentialTestGatewayClient(t)
	entry := staticOAuthTestEntry("entry-1", "default", provider.URL+"/mcp")
	handler := &MCPCatalogHandler{
		serverURL:     "https://obot.example",
		gatewayClient: gateway,
		remoteURLValidationConfig: mcp.RemoteMCPURLValidationConfig{
			AllowLocalhostMCP: true,
			AllowPrivateIPMCP: true,
			AllowLinkLocalMCP: true,
		},
	}

	startRecorder := httptest.NewRecorder()
	startReq := newStaticOAuthTestRequest(t, http.MethodPost, `/`, `{"clientID":"  static-client  ","clientSecret":"  static-secret  "}`, startRecorder, gateway,
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
	startReq.SetPathValue("catalog_id", "default")
	startReq.SetPathValue("entry_id", entry.Name)

	if err := handler.StartOAuthCredentialTest(startReq); err != nil {
		t.Fatalf("start static OAuth credential test: %v", err)
	}
	var started map[string]string
	if err := json.Unmarshal(startRecorder.Body.Bytes(), &started); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if len(started) != 2 || started["testState"] == "" || started["oauthURL"] == "" {
		t.Fatalf("start response = %#v, want only testState and oauthURL", started)
	}
	for _, sensitive := range []string{"static-secret", provider.URL + "/token"} {
		if strings.Contains(startRecorder.Body.String(), sensitive) {
			t.Fatalf("start response exposed sensitive value %q: %s", sensitive, startRecorder.Body.String())
		}
	}

	authURL, err := url.Parse(started["oauthURL"])
	if err != nil {
		t.Fatalf("parse OAuth URL: %v", err)
	}
	if authURL.Query().Get("client_id") != "  static-client  " || authURL.Query().Get("redirect_uri") != "https://obot.example/oauth/mcp/callback" {
		t.Fatalf("OAuth URL query = %s", authURL.RawQuery)
	}
	if authURL.Query().Get("code_challenge") == "" || authURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("OAuth URL does not require PKCE: %s", authURL.RawQuery)
	}
	if authURL.Query().Get("state") == "" || authURL.Query().Get("state") == started["testState"] {
		t.Fatalf("callback state was missing or reused the body-only test state: %s", authURL.RawQuery)
	}
	authResponse, err := http.Get(started["oauthURL"])
	if err != nil {
		t.Fatalf("open provider authorization URL: %v", err)
	}
	_ = authResponse.Body.Close()
	if authResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("provider rejected authorization URL with status %d", authResponse.StatusCode)
	}

	statusRecorder := httptest.NewRecorder()
	statusReq := newStaticOAuthTestRequest(t, http.MethodPost, `/`, staticOAuthStatusBody(t, started["testState"]), statusRecorder, gateway,
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
	statusReq.SetPathValue("catalog_id", "default")
	statusReq.SetPathValue("entry_id", entry.Name)
	if err := handler.GetOAuthCredentialTest(statusReq); err != nil {
		t.Fatalf("get static OAuth credential test: %v", err)
	}
	var status types.MCPStaticOAuthTestResult
	if err := json.Unmarshal(statusRecorder.Body.Bytes(), &status); err != nil {
		t.Fatalf("decode pending status response: %v", err)
	}
	if status.Status != types.MCPStaticOAuthTestStatusPending || status.ExpiresAt.IsZero() || !status.ExpiresAt.Time.After(time.Now()) {
		t.Fatalf("status response = %+v, want safe pending status with future expiry", status)
	}
	for _, sensitive := range []string{"static-secret", provider.URL + "/token"} {
		if strings.Contains(statusRecorder.Body.String(), sensitive) {
			t.Fatalf("status response exposed sensitive value %q: %s", sensitive, statusRecorder.Body.String())
		}
	}

	wrongCallerReq := newStaticOAuthTestRequest(t, http.MethodPost, `/`, staticOAuthStatusBody(t, started["testState"]), httptest.NewRecorder(), gateway,
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
	wrongCallerReq.User = &user.DefaultInfo{Name: "other", UID: "user-2"}
	wrongCallerReq.SetPathValue("catalog_id", "default")
	wrongCallerReq.SetPathValue("entry_id", entry.Name)
	if err := handler.GetOAuthCredentialTest(wrongCallerReq); err == nil {
		t.Fatal("wrong caller read static OAuth test status")
	}
}

func TestFailedClearRefreshCannotResurrectAfterSuccessfulRetry(t *testing.T) {
	const (
		entryName = "entry-1"
		mcpID     = "instance-1"
		mcpURL    = "https://mcp.example/api"
	)
	entry := staticOAuthTestEntry(entryName, "default", mcpURL)
	instance := &v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{Name: mcpID, Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerInstanceSpec{
			UserID:                    "user-1",
			MCPServerCatalogEntryName: entryName,
		},
	}
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithIndex(&v1.MCPServer{}, "spec.mcpServerCatalogEntryName", func(object client.Object) []string {
			return []string{object.(*v1.MCPServer).Spec.MCPServerCatalogEntryName}
		}).
		WithIndex(&v1.MCPServerInstance{}, "spec.mcpServerCatalogEntryName", func(object client.Object) []string {
			return []string{object.(*v1.MCPServerInstance).Spec.MCPServerCatalogEntryName}
		}).
		WithObjects(
			&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}},
			entry,
			instance,
		).
		Build()
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("create storage services: %v", err)
	}
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("create gateway database: %v", err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("migrate gateway database: %v", err)
	}
	gateway := gatewayclient.New(t.Context(), database, storageClient, nil, nil, nil, nil, time.Hour, 10, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	credentialKey := system.MCPOAuthCredentialName(entryName)
	if err := gateway.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: credentialKey, Name: "oauth",
		Secrets: map[string]string{
			"CLIENT_ID": "client-1", "CLIENT_SECRET": "secret-1",
			"MCP_URL": mcpURL, "GENERATION": "generation-1",
		},
	}); err != nil {
		t.Fatalf("seed catalog credential: %v", err)
	}
	config := &oauth2.Config{ClientID: "client-1", ClientSecret: "secret-1"}
	if err := gateway.ReplaceMCPOAuthTokenWithCatalogCredentialGenerationFence(
		t.Context(), "user-1", mcpID, mcpURL, "", entryName, "generation-1", config,
		&oauth2.Token{AccessToken: "active-access", RefreshToken: "active-refresh"},
	); err != nil {
		t.Fatalf("seed active token: %v", err)
	}

	newDeleteRequest := func(client storage.Client) api.Context {
		req := api.Context{
			Request: httptest.NewRequest(
				http.MethodDelete, "/", strings.NewReader(`{"expectedGeneration":"generation-1"}`),
			),
			ResponseWriter: httptest.NewRecorder(),
			Storage:        client,
			GatewayClient:  gateway,
			User:           &user.DefaultInfo{Name: "owner", UID: "user-1"},
		}
		req.SetPathValue("catalog_id", "default")
		req.SetPathValue("entry_id", entryName)
		return req
	}
	handler := &MCPCatalogHandler{gatewayClient: gateway}
	failedRequest := newDeleteRequest(oauthServerListErrorStorage{Client: storage.Client(storageClient)})
	if err := handler.DeleteOAuthCredentials(failedRequest); err == nil {
		t.Fatal("first Clear succeeded despite target list failure")
	}
	if _, err := gateway.GetMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL); err != nil {
		t.Fatalf("failed Clear removed active token before retry: %v", err)
	}

	store := mcpgateway.NewGlobalTokenStore(gateway).ForUserAndMCP("user-1", mcpID)
	if _, _, err := store.GetTokenConfig(t.Context(), mcpURL); err != nil {
		t.Fatalf("refresh after failed Clear: %v", err)
	}
	refreshStarted := make(chan struct{})
	writeRefresh := make(chan struct{})
	refreshResult := make(chan error, 1)
	go func() {
		close(refreshStarted)
		<-writeRefresh
		refreshResult <- store.SetTokenConfig(t.Context(), mcpURL, config, &oauth2.Token{AccessToken: "resurrected-access"})
	}()
	<-refreshStarted
	if err := handler.DeleteOAuthCredentials(newDeleteRequest(storage.Client(storageClient))); err != nil {
		t.Fatalf("Clear retry failed: %v", err)
	}
	close(writeRefresh)
	if err := <-refreshResult; !errors.Is(err, gatewayclient.ErrMCPOAuthCatalogCredentialChanged) {
		t.Fatalf("in-flight refresh write error = %v, want credential changed", err)
	}
	if _, err := gateway.GetMCPOAuthToken(t.Context(), "user-1", mcpID, mcpURL); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("legacy token resurrected after successful Clear retry: %v", err)
	}
}

func TestStartOAuthCredentialTestRejectsWrongScopeShapeCandidatesAndBlockedURL(t *testing.T) {
	provider := newStaticOAuthTestProvider(t)
	for _, tt := range []struct {
		name       string
		body       string
		configure  func(*v1.MCPServerCatalogEntry)
		allowLocal bool
	}{
		{name: "catalog scope mismatch", body: "{\"clientID\":\"client\",\"clientSecret\":\"secret\"}", configure: func(entry *v1.MCPServerCatalogEntry) { entry.Spec.MCPCatalogName = "other" }, allowLocal: true},
		{name: "single user", body: "{\"clientID\":\"client\",\"clientSecret\":\"secret\"}", configure: func(entry *v1.MCPServerCatalogEntry) {
			entry.Spec.Manifest.ServerUserType = types.ServerUserTypeSingleUser
		}, allowLocal: true},
		{name: "not remote", body: "{\"clientID\":\"client\",\"clientSecret\":\"secret\"}", configure: func(entry *v1.MCPServerCatalogEntry) { entry.Spec.Manifest.Runtime = types.RuntimeNPX }, allowLocal: true},
		{name: "missing fixed URL", body: "{\"clientID\":\"client\",\"clientSecret\":\"secret\"}", configure: func(entry *v1.MCPServerCatalogEntry) { entry.Spec.Manifest.RemoteConfig.FixedURL = "" }, allowLocal: true},
		{name: "static OAuth not required", body: "{\"clientID\":\"client\",\"clientSecret\":\"secret\"}", configure: func(entry *v1.MCPServerCatalogEntry) { entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired = false }, allowLocal: true},
		{name: "blank client ID", body: "{\"clientID\":\"  \",\"clientSecret\":\"secret\"}", allowLocal: true},
		{name: "blank client secret", body: "{\"clientID\":\"client\",\"clientSecret\":\"  \"}", allowLocal: true},
		{name: "blocked local URL", body: "{\"clientID\":\"client\",\"clientSecret\":\"secret\"}"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gateway := newOAuthCredentialTestGatewayClient(t)
			entry := staticOAuthTestEntry("entry-1", "default", provider.URL+"/mcp")
			if tt.configure != nil {
				tt.configure(entry)
			}
			handler := &MCPCatalogHandler{serverURL: "https://obot.example", gatewayClient: gateway}
			if tt.allowLocal {
				handler.remoteURLValidationConfig = mcp.RemoteMCPURLValidationConfig{AllowLocalhostMCP: true, AllowPrivateIPMCP: true, AllowLinkLocalMCP: true}
			}
			req := newStaticOAuthTestRequest(t, http.MethodPost, "/", tt.body, httptest.NewRecorder(), gateway,
				&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
			req.SetPathValue("catalog_id", "default")
			req.SetPathValue("entry_id", entry.Name)
			if err := handler.StartOAuthCredentialTest(req); err == nil {
				t.Fatal("invalid static OAuth test start succeeded")
			}
		})
	}
}

func TestStartOAuthCredentialTestRejectsUnsafeDiscoveredEndpoints(t *testing.T) {
	for _, tt := range []struct {
		name                  string
		authorizationEndpoint string
		tokenEndpoint         string
	}{
		{name: "script authorization endpoint", authorizationEndpoint: "javascript:alert(document.domain)", tokenEndpoint: "https://provider.example/token"},
		{name: "private token endpoint", authorizationEndpoint: "https://provider.example/authorize", tokenEndpoint: "http://127.0.0.1/token"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			provider := newStaticOAuthMetadataProvider(t, tt.authorizationEndpoint, tt.tokenEndpoint)
			gateway := newOAuthCredentialTestGatewayClient(t)
			entry := staticOAuthTestEntry("entry-1", "default", provider.URL+"/mcp")
			handler := &MCPCatalogHandler{
				serverURL:     "https://obot.example",
				gatewayClient: gateway,
				remoteURLValidationConfig: mcp.RemoteMCPURLValidationConfig{
					AllowLocalhostMCP: true,
					AllowPrivateIPMCP: false,
					AllowLinkLocalMCP: false,
				},
			}
			req := newStaticOAuthTestRequest(t, http.MethodPost, "/", `{"clientID":"client","clientSecret":"secret"}`, httptest.NewRecorder(), gateway,
				&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
			req.SetPathValue("catalog_id", "default")
			req.SetPathValue("entry_id", entry.Name)
			if err := handler.StartOAuthCredentialTest(req); err == nil {
				t.Fatal("unsafe discovered OAuth endpoint was accepted")
			}
		})
	}
}

func TestGetOAuthCredentialTestProjectsSafeCompletedStatusesAndEntryIsolation(t *testing.T) {
	for _, tt := range []struct {
		name        string
		status      types.MCPStaticOAuthTestStatus
		failure     types.MCPStaticOAuthTestFailureCategory
		wantStatus  string
		wantFailure string
	}{
		{name: "succeeded", status: types.MCPStaticOAuthTestStatusSucceeded, wantStatus: "succeeded"},
		{name: "failed", status: types.MCPStaticOAuthTestStatusFailed, failure: types.MCPStaticOAuthTestFailureAuthorizationDenied, wantStatus: "failed", wantFailure: "authorization_denied"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gateway := newOAuthCredentialTestGatewayClient(t)
			entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
			state := pendingStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
			if err := gateway.CompleteMCPStaticOAuthTest(t.Context(), state.CallbackState, tt.status, tt.failure); err != nil {
				t.Fatalf("complete proof: %v", err)
			}
			recorder := httptest.NewRecorder()
			req := newStaticOAuthTestRequest(t, http.MethodPost, "/", staticOAuthStatusBody(t, state.TestState), recorder, gateway,
				&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
			req.SetPathValue("catalog_id", "default")
			req.SetPathValue("entry_id", entry.Name)
			if err := (&MCPCatalogHandler{gatewayClient: gateway}).GetOAuthCredentialTest(req); err != nil {
				t.Fatalf("get proof status: %v", err)
			}
			var got map[string]string
			if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode status: %v", err)
			}
			if got["status"] != tt.wantStatus || got["failureCategory"] != tt.wantFailure {
				t.Fatalf("status = %#v", got)
			}
			for _, sensitive := range []string{"candidate-client", "candidate-secret", "verifier", "mcp.example", "provider.example"} {
				if strings.Contains(recorder.Body.String(), sensitive) {
					t.Fatalf("status exposed %q: %s", sensitive, recorder.Body.String())
				}
			}
		})
	}

	gateway := newOAuthCredentialTestGatewayClient(t)
	entryOne := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	entryTwo := staticOAuthTestEntry("entry-2", "default", "https://mcp.example/api")
	state := pendingStaticOAuthCredentialProof(t, gateway, entryOne.Name, entryOne.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
	req := newStaticOAuthTestRequest(t, http.MethodPost, "/", staticOAuthStatusBody(t, state.TestState), httptest.NewRecorder(), gateway,
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entryTwo)
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entryTwo.Name)
	if err := (&MCPCatalogHandler{gatewayClient: gateway}).GetOAuthCredentialTest(req); err == nil {
		t.Fatal("proof status crossed entry boundary")
	}
}

func TestGetOAuthCredentialTestProjectsExpiredStatusAtHandlerBoundary(t *testing.T) {
	gateway, rawDB := newOAuthCredentialTestGatewayClientWithOptionsAndDB(t, nil, nil)
	entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
	state := pendingStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
	if err := rawDB.Model(&gatewaytypes.MCPOAuthPendingState{}).
		Where("static_o_auth_test = ?", true).
		Update("created_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatalf("expire pending proof: %v", err)
	}

	recorder := httptest.NewRecorder()
	req := newStaticOAuthTestRequest(t, http.MethodPost, "/", staticOAuthStatusBody(t, state.TestState), recorder, gateway,
		&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
	req.SetPathValue("catalog_id", "default")
	req.SetPathValue("entry_id", entry.Name)
	if err := (&MCPCatalogHandler{gatewayClient: gateway}).GetOAuthCredentialTest(req); err != nil {
		t.Fatalf("get expired proof status: %v", err)
	}
	var result types.MCPStaticOAuthTestResult
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode expired status response: %v", err)
	}
	if result.Status != types.MCPStaticOAuthTestStatusFailed || result.FailureCategory != types.MCPStaticOAuthTestFailureExpired || result.ExpiresAt.IsZero() || !result.ExpiresAt.Time.Before(time.Now()) {
		t.Fatalf("expired status response = %+v", result)
	}
}

func TestGetOAuthCredentialTestReturnsSafeBadRequestForUnavailableProof(t *testing.T) {
	for _, tt := range []struct {
		name    string
		prepare func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string
	}{
		{
			name: "unknown",
			prepare: func(*testing.T, *gatewayclient.Client, *v1.MCPServerCatalogEntry) string {
				return "unknown-proof"
			},
		},
		{
			name: "consumed",
			prepare: func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string {
				t.Helper()
				started := pendingStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
				if err := gateway.CompleteMCPStaticOAuthTest(t.Context(), started.CallbackState, types.MCPStaticOAuthTestStatusSucceeded, ""); err != nil {
					t.Fatalf("complete proof: %v", err)
				}
				result, err := gateway.GetMCPStaticOAuthTestStatus(t.Context(), started.TestState, "user-1", entry.Name)
				if err != nil {
					t.Fatalf("read proof: %v", err)
				}
				if err := commitMCPStaticOAuthCredential(t.Context(), gateway, result.Proof, "user-1", entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "candidate-client", "candidate-secret", false); err != nil {
					t.Fatalf("consume proof through commit: %v", err)
				}
				return started.TestState
			},
		},
		{
			name: "surrounding whitespace is not normalized",
			prepare: func(t *testing.T, gateway *gatewayclient.Client, entry *v1.MCPServerCatalogEntry) string {
				t.Helper()
				started := pendingStaticOAuthCredentialProof(t, gateway, entry.Name, entry.Spec.Manifest.RemoteConfig.FixedURL, "user-1")
				return " " + started.TestState + " "
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gateway := newOAuthCredentialTestGatewayClient(t)
			entry := staticOAuthTestEntry("entry-1", "default", "https://mcp.example/api")
			proof := tt.prepare(t, gateway, entry)
			req := newStaticOAuthTestRequest(t, http.MethodPost, "/", staticOAuthStatusBody(t, proof), httptest.NewRecorder(), gateway,
				&v1.MCPCatalog{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: system.DefaultNamespace}}, entry)
			req.SetPathValue("catalog_id", "default")
			req.SetPathValue("entry_id", entry.Name)

			err := (&MCPCatalogHandler{gatewayClient: gateway}).GetOAuthCredentialTest(req)
			var httpErr *types.ErrHTTP
			if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest || httpErr.Message != "invalid or expired OAuth credential test" {
				t.Fatalf("unavailable proof error = %#v, want safe HTTP 400", err)
			}
		})
	}
}

func staticOAuthTestEntry(name, catalogName, fixedURL string) *v1.MCPServerCatalogEntry {
	return &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: catalogName,
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime:        types.RuntimeRemote,
				ServerUserType: types.ServerUserTypeMultiUser,
				RemoteConfig: &types.RemoteCatalogConfig{
					FixedURL:            fixedURL,
					StaticOAuthRequired: true,
				},
			},
		},
	}
}

func newStaticOAuthTestRequest(t *testing.T, method, target, body string, recorder *httptest.ResponseRecorder, gateway *gatewayclient.Client, objects ...client.Object) api.Context {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	return api.Context{
		Request:        req,
		ResponseWriter: recorder,
		Storage: storage.Client(fake.NewClientBuilder().
			WithScheme(storagescheme.Scheme).
			WithObjects(objects...).
			Build()),
		GatewayClient: gateway,
		User:          &user.DefaultInfo{Name: "owner", UID: "user-1"},
	}
}

func staticOAuthStatusBody(t *testing.T, state string) string {
	t.Helper()
	body, err := json.Marshal(types.MCPServerOAuthCredentialTestStatusRequest{TestState: state})
	if err != nil {
		t.Fatalf("marshal status request: %v", err)
	}
	return string(body)
}

func newOAuthCredentialTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	return newOAuthCredentialTestGatewayClientWithOptions(t, staticOAuthTestEncryptionConfig(), nil)
}

func newOAuthCredentialTestGatewayClientWithEncryption(t *testing.T, encryptionConfig *encryptionconfig.EncryptionConfiguration) *gatewayclient.Client {
	t.Helper()
	return newOAuthCredentialTestGatewayClientWithOptions(t, encryptionConfig, nil)
}

func newOAuthCredentialTestGatewayClientWithTrigger(t *testing.T, trigger func(context.Context, string) error) *gatewayclient.Client {
	t.Helper()
	return newOAuthCredentialTestGatewayClientWithOptions(t, staticOAuthTestEncryptionConfig(), trigger)
}

func newOAuthCredentialTestGatewayClientWithOptions(t *testing.T, encryptionConfig *encryptionconfig.EncryptionConfiguration, trigger func(context.Context, string) error) *gatewayclient.Client {
	t.Helper()
	if encryptionConfig == nil {
		encryptionConfig = staticOAuthTestEncryptionConfig()
	} else {
		if encryptionConfig.Transformers == nil {
			encryptionConfig.Transformers = map[schema.GroupResource]value.Transformer{}
		}
		pendingResource := schema.GroupResource{Group: "obot.obot.ai", Resource: "mcpoauthpendingstates"}
		credentialResource := schema.GroupResource{Group: "obot.obot.ai", Resource: "credentials"}
		if encryptionConfig.Transformers[pendingResource] == nil {
			encryptionConfig.Transformers[pendingResource] = &toggleCredentialWriteErrorTransformer{}
		}
		if encryptionConfig.Transformers[credentialResource] == nil {
			encryptionConfig.Transformers[credentialResource] = &toggleCredentialWriteErrorTransformer{}
		}
	}
	client, _ := newOAuthCredentialTestGatewayClientWithOptionsAndDB(t, encryptionConfig, trigger)
	return client
}

func staticOAuthTestEncryptionConfig() *encryptionconfig.EncryptionConfiguration {
	transformer := &toggleCredentialWriteErrorTransformer{}
	return &encryptionconfig.EncryptionConfiguration{Transformers: map[schema.GroupResource]value.Transformer{
		{Group: "obot.obot.ai", Resource: "mcpoauthpendingstates"}: transformer,
		{Group: "obot.obot.ai", Resource: "credentials"}:           transformer,
	}}
}

func newOAuthCredentialTestGatewayClientWithOptionsAndDB(t *testing.T, encryptionConfig *encryptionconfig.EncryptionConfiguration, trigger func(context.Context, string) error) (*gatewayclient.Client, *gorm.DB) {
	t.Helper()
	if encryptionConfig == nil {
		encryptionConfig = staticOAuthTestEncryptionConfig()
	}
	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("create storage services: %v", err)
	}
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("create gateway database: %v", err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("migrate gateway database: %v", err)
	}
	return gatewayclient.New(t.Context(), database, nil, encryptionConfig, trigger, nil, nil, time.Hour, 10, 0, 0, false), services.DB.DB
}

func newStaticOAuthTestProvider(t *testing.T) *httptest.Server {
	t.Helper()
	var providerURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/mcp":
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource/mcp":
			http.NotFound(w, req)
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              providerURL + "/mcp",
				"authorization_servers": []string{providerURL},
			})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer":                                providerURL,
				"authorization_endpoint":                providerURL + "/authorize",
				"token_endpoint":                        providerURL + "/token",
				"response_types_supported":              []string{"code"},
				"code_challenge_methods_supported":      []string{"S256"},
				"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
			})
		case "/authorize":
			if req.URL.Query().Get("client_id") != "  static-client  " || req.URL.Query().Get("code_challenge") == "" || req.URL.Query().Get("code_challenge_method") != "S256" {
				http.Error(w, "invalid authorization request", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		case "/token":
			clientID, clientSecret, ok := req.BasicAuth()
			if !ok || clientID != "  static-client  " || clientSecret != "  static-secret  " || req.FormValue("code_verifier") == "" {
				http.Error(w, "invalid token request", http.StatusUnauthorized)
				return
			}
			_ = json.NewEncoder(w).Encode(&oauth2.Token{AccessToken: "discard-me", TokenType: "Bearer"})
		default:
			http.NotFound(w, req)
		}
	}))
	providerURL = server.URL
	t.Cleanup(server.Close)
	return server
}

func newStaticOAuthMetadataProvider(t *testing.T, authorizationEndpoint, tokenEndpoint string) *httptest.Server {
	t.Helper()
	var providerURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/mcp":
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		case "/.well-known/oauth-protected-resource/mcp":
			http.NotFound(w, req)
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": providerURL + "/mcp", "authorization_servers": []string{providerURL}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": providerURL, "authorization_endpoint": authorizationEndpoint, "token_endpoint": tokenEndpoint,
				"response_types_supported": []string{"code"}, "code_challenge_methods_supported": []string{"S256"},
			})
		default:
			http.NotFound(w, req)
		}
	}))
	providerURL = server.URL
	t.Cleanup(server.Close)
	return server
}
