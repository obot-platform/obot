package internal

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrg/xdg"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/cli/internal/credentials"
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
	got, err := AppURLForAPIBaseURL("https://obot.example.com/api/")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://obot.example.com" {
		t.Fatalf("expected app URL, got %q", got)
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

	token, err := Token(t.Context(), serverURL+"/api", false, false)
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

	if _, err := Token(t.Context(), srv.URL+"/api", false, false); !errors.Is(err, keyringErr) {
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

	if _, err := Token(t.Context(), srv.URL+"/api", false, false); err == nil || !strings.Contains(err.Error(), "no auth providers") {
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
					Name:      "GitHub",
					Namespace: "default",
				},
				AuthProviderStatus: types.AuthProviderStatus{
					CommonProviderStatus: types.CommonProviderStatus{Configured: true},
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

	token, err := Token(t.Context(), srv.URL+"/api", false, true)
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

func TestLogoutDeletesSelectedAppURLToken(t *testing.T) {
	store := newFakeCredentialStore()
	store.tokens["https://obot.example.com"] = "token-a"
	store.tokens["https://other.example.com"] = "token-b"
	restore := useCredentialStore(t, store)
	defer restore()

	if err := Logout("https://obot.example.com/"); err != nil {
		t.Fatal(err)
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

	if err := Logout("https://obot.example.com"); err != nil {
		t.Fatal(err)
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

	_, _ = Token(t.Context(), srv.URL+"/api", false, false)
	if usedLegacyToken {
		t.Fatalf("legacy token file should be ignored")
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
