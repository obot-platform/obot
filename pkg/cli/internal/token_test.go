package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/cli/internal/credentials"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

type fakeCredentialStore struct {
	tokens  map[string]string
	err     error
	deleted []string
}

func newFakeCredentialStore() *fakeCredentialStore {
	return &fakeCredentialStore{tokens: map[string]string{}}
}

func (f *fakeCredentialStore) Get(appURL string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	token, ok := f.tokens[appURL]
	if !ok {
		return "", credentials.ErrNotFound
	}
	return token, nil
}

func (f *fakeCredentialStore) Set(appURL, token string) error {
	if f.err != nil {
		return f.err
	}
	f.tokens[appURL] = token
	return nil
}

func (f *fakeCredentialStore) Delete(appURL string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.tokens, appURL)
	f.deleted = append(f.deleted, appURL)
	return nil
}

func TestAppURLForAPIBaseURL(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "app URL",
			baseURL: "https://obot.example.com",
			want:    "https://obot.example.com",
		},
		{
			name:    "API URL",
			baseURL: "https://obot.example.com/api",
			want:    "https://obot.example.com",
		},
		{
			name:    "API URL trailing slash",
			baseURL: "https://obot.example.com/api/",
			want:    "https://obot.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AppURLForAPIBaseURL(tt.baseURL)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("expected app URL %q, got %q", tt.want, got)
			}
		})
	}
}

func TestTokenUsesKeyringTokenScopedByAppURL(t *testing.T) {
	store := newFakeCredentialStore()
	restore := useCredentialStore(t, store)
	defer restore()

	var serverURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/me" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		switch r.Header.Get("Authorization") {
		case "":
			_ = json.NewEncoder(w).Encode(types.User{Username: "anonymous"})
		case "Bearer keyring-token":
			_ = json.NewEncoder(w).Encode(types.User{Username: "alice"})
		default:
			t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
		}
	}))
	defer srv.Close()
	serverURL = srv.URL
	store.tokens[serverURL] = "keyring-token"

	token, err := Token(t.Context(), serverURL+"/api", apiclient.TokenFetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if token != "keyring-token" {
		t.Fatalf("expected keyring token, got %q", token)
	}
}

func TestTokenKeyringErrorFailsClosed(t *testing.T) {
	keyringErr := errors.New("keyring unavailable")
	store := newFakeCredentialStore()
	store.err = keyringErr
	restore := useCredentialStore(t, store)
	defer restore()

	var authProvidersCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me":
			_ = json.NewEncoder(w).Encode(types.User{Username: "anonymous"})
		case "/api/auth-providers":
			authProvidersCalled = true
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	if _, err := Token(t.Context(), srv.URL+"/api", apiclient.TokenFetchOptions{}); !errors.Is(err, keyringErr) {
		t.Fatalf("expected keyring error, got %v", err)
	}
	if authProvidersCalled {
		t.Fatalf("auth provider discovery should not run after keyring error")
	}
}

func TestTokenNotFoundTriggersLoginPath(t *testing.T) {
	store := newFakeCredentialStore()
	restore := useCredentialStore(t, store)
	defer restore()

	authProvidersCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me":
			_ = json.NewEncoder(w).Encode(types.User{Username: "anonymous"})
		case "/api/auth-providers":
			authProvidersCalled = true
			_ = json.NewEncoder(w).Encode(types.AuthProviderList{})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	if _, err := Token(t.Context(), srv.URL+"/api", apiclient.TokenFetchOptions{}); err == nil || !strings.Contains(err.Error(), "no auth providers") {
		t.Fatalf("expected auth provider error, got %v", err)
	}
	if !authProvidersCalled {
		t.Fatalf("expected auth provider discovery after missing keyring token")
	}
}

func TestTokenStoresNewTokenByAppURL(t *testing.T) {
	store := newFakeCredentialStore()
	restoreStore := useCredentialStore(t, store)
	defer restoreStore()

	restoreBrowser := useOpenBrowser(t, func(string) error { return nil })
	defer restoreBrowser()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me":
			_ = json.NewEncoder(w).Encode(types.User{Username: "anonymous"})
		case "/api/auth-providers":
			_ = json.NewEncoder(w).Encode(types.AuthProviderList{Items: []types.AuthProvider{{
				Metadata: types.Metadata{ID: "github"},
				AuthProviderManifest: types.AuthProviderManifest{
					CommonProviderMetadata: types.CommonProviderMetadata{
						Name: "GitHub",
					},
				},
				AuthProviderStatus: types.AuthProviderStatus{
					CommonProviderStatus: types.CommonProviderStatus{Configured: true},
					Namespace:            "default",
				},
			}}})
		case "/api/token-request":
			_ = json.NewEncoder(w).Encode(map[string]string{"token-path": "https://example.com/login"})
		default:
			if strings.HasPrefix(r.URL.Path, "/api/token-request/") {
				_ = json.NewEncoder(w).Encode(map[string]string{"Token": "new-token"})
				return
			}
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	store.tokens[srv.URL] = "expired-token"

	token, err := Token(t.Context(), srv.URL+"/api", apiclient.TokenFetchOptions{ForceRefresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if token != "new-token" {
		t.Fatalf("expected new token, got %q", token)
	}
	if got := store.tokens[srv.URL]; got != "new-token" {
		t.Fatalf("expected token stored by app URL, got %q", got)
	}
	if _, ok := store.tokens[srv.URL+"/api"]; ok {
		t.Fatalf("token should not be stored by API URL")
	}
}

func TestTokenRequestIncludesRequestedScopes(t *testing.T) {
	tests := []struct {
		name         string
		scopes       []string
		noExpiration bool
		want         createRequest
	}{
		{
			name:   "API scope",
			scopes: []string{"api"},
			want: createRequest{
				Scopes: gatewaytypes.APIKeyScopes{CanAccessAPI: true},
			},
		},
		{
			name:   "LLM scope",
			scopes: []string{"llm"},
			want: createRequest{
				Scopes: gatewaytypes.APIKeyScopes{CanAccessLLMProxy: true},
			},
		},
		{
			name:   "skills scope",
			scopes: []string{"skills"},
			want: createRequest{
				Scopes: gatewaytypes.APIKeyScopes{CanAccessSkills: true},
			},
		},
		{
			name:         "all MCP scope and no expiration",
			scopes:       []string{"all-mcp"},
			noExpiration: true,
			want: createRequest{
				NoExpiration: true,
				Scopes:       gatewaytypes.APIKeyScopes{MCPServerIDs: []string{"*"}},
			},
		},
		{
			name:   "combined scopes",
			scopes: []string{"api", "llm", "skills", "all-mcp"},
			want: createRequest{
				Scopes: gatewaytypes.APIKeyScopes{
					CanAccessAPI:      true,
					CanAccessLLMProxy: true,
					CanAccessSkills:   true,
					MCPServerIDs:      []string{"*"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeCredentialStore()
			restoreStore := useCredentialStore(t, store)
			defer restoreStore()

			restoreBrowser := useOpenBrowser(t, func(string) error { return nil })
			defer restoreBrowser()

			var got createRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/me":
					_ = json.NewEncoder(w).Encode(types.User{Username: "anonymous"})
				case "/api/auth-providers":
					_ = json.NewEncoder(w).Encode(types.AuthProviderList{Items: []types.AuthProvider{{
						Metadata: types.Metadata{ID: "github"},
						AuthProviderManifest: types.AuthProviderManifest{
							CommonProviderMetadata: types.CommonProviderMetadata{
								Name: "GitHub",
							},
						},
						AuthProviderStatus: types.AuthProviderStatus{
							CommonProviderStatus: types.CommonProviderStatus{Configured: true},
							Namespace:            "default",
						},
					}}})
				case "/api/token-request":
					if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
						t.Fatalf("decode token request: %v", err)
					}
					_ = json.NewEncoder(w).Encode(map[string]string{"token-path": "https://example.com/login"})
				default:
					if strings.HasPrefix(r.URL.Path, "/api/token-request/") {
						_ = json.NewEncoder(w).Encode(map[string]string{"Token": "new-token"})
						return
					}
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
			}))
			defer srv.Close()

			_, err := Token(WithNonInteractive(t.Context()), srv.URL+"/api", apiclient.TokenFetchOptions{
				NoExpiration: tt.noExpiration,
				Scopes:       tt.scopes,
			})
			if err != nil {
				t.Fatal(err)
			}

			if got.ProviderName != "github" {
				t.Fatalf("providerName = %q, want github", got.ProviderName)
			}
			if got.ProviderNamespace != "default" {
				t.Fatalf("providerNamespace = %q, want default", got.ProviderNamespace)
			}
			if got.ID == "" {
				t.Fatal("expected generated token request ID")
			}
			assertCreateRequestScopes(t, got, tt.want)
		})
	}
}

func TestTokenRefreshesValidKeyringTokenMissingRequestedScopes(t *testing.T) {
	store := newFakeCredentialStore()
	restoreStore := useCredentialStore(t, store)
	defer restoreStore()

	restoreBrowser := useOpenBrowser(t, func(string) error { return nil })
	defer restoreBrowser()

	var createdRequest createRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me":
			switch r.Header.Get("Authorization") {
			case "":
				_ = json.NewEncoder(w).Encode(types.User{Username: "anonymous"})
			case "Bearer scoped-token":
				_ = json.NewEncoder(w).Encode(types.User{Username: "alice"})
			default:
				t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
			}
		case "/api/api-keys/auth":
			if r.Header.Get("Authorization") != "Bearer scoped-token" {
				t.Fatalf("unexpected scope validation authorization header %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"allowed": true,
				"scopes": map[string]any{
					"canAccessSkills": true,
				},
			})
		case "/api/auth-providers":
			_ = json.NewEncoder(w).Encode(types.AuthProviderList{Items: []types.AuthProvider{{
				Metadata: types.Metadata{ID: "github"},
				AuthProviderManifest: types.AuthProviderManifest{
					CommonProviderMetadata: types.CommonProviderMetadata{
						Name: "GitHub",
					},
				},
				AuthProviderStatus: types.AuthProviderStatus{
					CommonProviderStatus: types.CommonProviderStatus{Configured: true},
					Namespace:            "default",
				},
			}}})
		case "/api/token-request":
			if err := json.NewDecoder(r.Body).Decode(&createdRequest); err != nil {
				t.Fatalf("decode token request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token-path": "https://example.com/login"})
		default:
			if strings.HasPrefix(r.URL.Path, "/api/token-request/") {
				_ = json.NewEncoder(w).Encode(map[string]string{"Token": "new-token"})
				return
			}
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	store.tokens[srv.URL] = "scoped-token"

	token, err := Token(WithNonInteractive(t.Context()), srv.URL+"/api", apiclient.TokenFetchOptions{
		Scopes: []string{"api"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if token != "new-token" {
		t.Fatalf("token = %q, want new token", token)
	}
	if got := store.tokens[srv.URL]; got != "new-token" {
		t.Fatalf("stored token = %q, want new token", got)
	}
	if !createdRequest.Scopes.CanAccessAPI {
		t.Fatalf("new token request should ask for API scope")
	}
}

func TestTokenReusesAPIKeyringTokenForNonMCPScopes(t *testing.T) {
	store := newFakeCredentialStore()
	restoreStore := useCredentialStore(t, store)
	defer restoreStore()

	authProvidersCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me":
			switch r.Header.Get("Authorization") {
			case "":
				_ = json.NewEncoder(w).Encode(types.User{Username: "anonymous"})
			case "Bearer api-token":
				_ = json.NewEncoder(w).Encode(types.User{Username: "alice"})
			default:
				t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
			}
		case "/api/api-keys/auth":
			if r.Header.Get("Authorization") != "Bearer api-token" {
				t.Fatalf("unexpected scope validation authorization header %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"allowed": true,
				"scopes": map[string]any{
					"canAccessAPI": true,
				},
			})
		case "/api/auth-providers":
			authProvidersCalled = true
			t.Fatalf("auth provider discovery should not run for reusable API token")
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	store.tokens[srv.URL] = "api-token"

	token, err := Token(WithNonInteractive(t.Context()), srv.URL+"/api", apiclient.TokenFetchOptions{
		Scopes: []string{"skills", "llm", "published-artifacts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if token != "api-token" {
		t.Fatalf("token = %q, want API token", token)
	}
	if authProvidersCalled {
		t.Fatalf("auth provider discovery should not run")
	}
}

func TestTokenRefreshesAPIKeyringTokenForMCPScope(t *testing.T) {
	store := newFakeCredentialStore()
	restoreStore := useCredentialStore(t, store)
	defer restoreStore()

	restoreBrowser := useOpenBrowser(t, func(string) error { return nil })
	defer restoreBrowser()

	var createdRequest createRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me":
			switch r.Header.Get("Authorization") {
			case "":
				_ = json.NewEncoder(w).Encode(types.User{Username: "anonymous"})
			case "Bearer api-token":
				_ = json.NewEncoder(w).Encode(types.User{Username: "alice"})
			default:
				t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
			}
		case "/api/api-keys/auth":
			if r.Header.Get("Authorization") != "Bearer api-token" {
				t.Fatalf("unexpected scope validation authorization header %q", r.Header.Get("Authorization"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"allowed": true,
				"scopes": map[string]any{
					"canAccessAPI": true,
				},
			})
		case "/api/auth-providers":
			_ = json.NewEncoder(w).Encode(types.AuthProviderList{Items: []types.AuthProvider{{
				Metadata: types.Metadata{ID: "github"},
				AuthProviderManifest: types.AuthProviderManifest{
					CommonProviderMetadata: types.CommonProviderMetadata{
						Name: "GitHub",
					},
				},
				AuthProviderStatus: types.AuthProviderStatus{
					CommonProviderStatus: types.CommonProviderStatus{Configured: true},
					Namespace:            "default",
				},
			}}})
		case "/api/token-request":
			if err := json.NewDecoder(r.Body).Decode(&createdRequest); err != nil {
				t.Fatalf("decode token request: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token-path": "https://example.com/login"})
		default:
			if strings.HasPrefix(r.URL.Path, "/api/token-request/") {
				_ = json.NewEncoder(w).Encode(map[string]string{"Token": "new-token"})
				return
			}
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	store.tokens[srv.URL] = "api-token"

	token, err := Token(WithNonInteractive(t.Context()), srv.URL+"/api", apiclient.TokenFetchOptions{
		Scopes: []string{"all-mcp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if token != "new-token" {
		t.Fatalf("token = %q, want new token", token)
	}
	if strings.Join(createdRequest.Scopes.MCPServerIDs, ",") != "*" {
		t.Fatalf("MCPServerIDs = %v, want [*]", createdRequest.Scopes.MCPServerIDs)
	}
}

func TestTokenNonInteractiveSkipsBrowserEnterGate(t *testing.T) {
	store := newFakeCredentialStore()
	restoreStore := useCredentialStore(t, store)
	defer restoreStore()

	var openedURL string
	restoreBrowser := useOpenBrowser(t, func(url string) error {
		openedURL = url
		return nil
	})
	defer restoreBrowser()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me":
			_ = json.NewEncoder(w).Encode(types.User{Username: "anonymous"})
		case "/api/auth-providers":
			_ = json.NewEncoder(w).Encode(types.AuthProviderList{Items: []types.AuthProvider{{
				Metadata: types.Metadata{ID: "github"},
				AuthProviderManifest: types.AuthProviderManifest{
					CommonProviderMetadata: types.CommonProviderMetadata{
						Name: "GitHub",
					},
				},
				AuthProviderStatus: types.AuthProviderStatus{
					CommonProviderStatus: types.CommonProviderStatus{Configured: true},
					Namespace:            "default",
				},
			}}})
		case "/api/token-request":
			_ = json.NewEncoder(w).Encode(map[string]string{"token-path": "https://example.com/login"})
		default:
			if strings.HasPrefix(r.URL.Path, "/api/token-request/") {
				_ = json.NewEncoder(w).Encode(map[string]string{"Token": "new-token"})
				return
			}
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	var output bytes.Buffer
	token, err := Token(WithOutputWriter(WithNonInteractive(t.Context()), &output), srv.URL+"/api", apiclient.TokenFetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if token != "new-token" {
		t.Fatalf("expected new token, got %q", token)
	}
	if openedURL != "https://example.com/login" {
		t.Fatalf("expected browser to open login URL, got %q", openedURL)
	}
	if !strings.Contains(output.String(), "Opening browser to https://example.com/login") {
		t.Fatalf("expected browser login message in configured output writer, got %q", output.String())
	}
	if got := store.tokens[srv.URL]; got != "new-token" {
		t.Fatalf("expected token stored by app URL, got %q", got)
	}
}

func TestStoredTokenValid(t *testing.T) {
	store := newFakeCredentialStore()
	restore := useCredentialStore(t, store)
	defer restore()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/me" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") == "Bearer valid-token" {
			_ = json.NewEncoder(w).Encode(types.User{Username: "alice"})
			return
		}
		_ = json.NewEncoder(w).Encode(types.User{Username: "anonymous"})
	}))
	defer srv.Close()

	valid, err := StoredTokenValid(t.Context(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatalf("token should not be valid when no stored token exists")
	}

	store.tokens[srv.URL] = "invalid-token"
	valid, err = StoredTokenValid(t.Context(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if valid {
		t.Fatalf("invalid stored token should not be valid")
	}

	store.tokens[srv.URL] = "valid-token"
	valid, err = StoredTokenValid(t.Context(), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if !valid {
		t.Fatalf("valid stored token should be valid")
	}
}

func TestLogoutDeletesSelectedAppURLToken(t *testing.T) {
	store := newFakeCredentialStore()
	store.tokens["https://obot.example.com"] = "token-a"
	store.tokens["https://other.example.com"] = "token-b"
	restore := useCredentialStore(t, store)
	defer restore()

	removed, err := Logout("https://obot.example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatalf("expected selected token to be removed")
	}
	if _, ok := store.tokens["https://obot.example.com"]; ok {
		t.Fatalf("expected selected token to be deleted")
	}
	if got := store.tokens["https://other.example.com"]; got != "token-b" {
		t.Fatalf("unexpected other token %q", got)
	}
}

func TestLogoutNotFound(t *testing.T) {
	store := newFakeCredentialStore()
	restore := useCredentialStore(t, store)
	defer restore()

	removed, err := Logout("https://obot.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatalf("expected no token to be removed")
	}
}

func TestLegacyTokenFileIsIgnored(t *testing.T) {
	store := newFakeCredentialStore()
	restore := useCredentialStore(t, store)
	defer restore()

	configHome := useTestXDGConfigHome(t)
	tokenFile := filepath.Join(configHome, "obot", "token")
	if err := os.MkdirAll(filepath.Dir(tokenFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tokenFile, []byte(`{"`+"legacy"+`":"old-token"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	usedLegacyToken := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/me":
			if r.Header.Get("Authorization") == "Bearer old-token" {
				usedLegacyToken = true
			}
			_ = json.NewEncoder(w).Encode(types.User{Username: "anonymous"})
		case "/api/auth-providers":
			_ = json.NewEncoder(w).Encode(types.AuthProviderList{})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	_, _ = Token(t.Context(), srv.URL+"/api", apiclient.TokenFetchOptions{})
	if usedLegacyToken {
		t.Fatalf("legacy token file should be ignored")
	}
}

func assertCreateRequestScopes(t *testing.T, got, want createRequest) {
	t.Helper()

	if got.NoExpiration != want.NoExpiration {
		t.Fatalf("NoExpiration = %v, want %v", got.NoExpiration, want.NoExpiration)
	}
	if got.Scopes.CanAccessAPI != want.Scopes.CanAccessAPI {
		t.Fatalf("CanAccessAPI = %v, want %v", got.Scopes.CanAccessAPI, want.Scopes.CanAccessAPI)
	}
	if got.Scopes.CanAccessLLMProxy != want.Scopes.CanAccessLLMProxy {
		t.Fatalf("CanAccessLLMProxy = %v, want %v", got.Scopes.CanAccessLLMProxy, want.Scopes.CanAccessLLMProxy)
	}
	if got.Scopes.CanAccessSkills != want.Scopes.CanAccessSkills {
		t.Fatalf("CanAccessSkills = %v, want %v", got.Scopes.CanAccessSkills, want.Scopes.CanAccessSkills)
	}
	if strings.Join(got.Scopes.MCPServerIDs, ",") != strings.Join(want.Scopes.MCPServerIDs, ",") {
		t.Fatalf("MCPServerIDs = %v, want %v", got.Scopes.MCPServerIDs, want.Scopes.MCPServerIDs)
	}
}

func useCredentialStore(t *testing.T, store credentials.Store) func() {
	t.Helper()
	previous := credentialStore
	credentialStore = store
	return func() {
		credentialStore = previous
	}
}

func useOpenBrowser(t *testing.T, fn func(string) error) func() {
	t.Helper()
	previous := openBrowser
	openBrowser = fn
	return func() {
		openBrowser = previous
	}
}

func useTestXDGConfigHome(t *testing.T) string {
	t.Helper()

	configHome := filepath.Join(t.TempDir(), "config")
	oldConfigHome, hadConfigHome := os.LookupEnv("XDG_CONFIG_HOME")
	if err := os.Setenv("XDG_CONFIG_HOME", configHome); err != nil {
		t.Fatal(err)
	}
	xdg.Reload()
	t.Cleanup(func() {
		if hadConfigHome {
			_ = os.Setenv("XDG_CONFIG_HOME", oldConfigHome)
		} else {
			_ = os.Unsetenv("XDG_CONFIG_HOME")
		}
		xdg.Reload()
	})
	return configHome
}
