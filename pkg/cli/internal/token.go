package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/obot-platform/obot/apiclient"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/cli/internal/credentials"
	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/pkg/browser"
)

var credentialStore credentials.Store = credentials.NewKeyringStore()
var openBrowser = browser.OpenURL

func init() {
	// Browser launchers (e.g. xdg-open) may write to stdout; keep stdout
	// reserved for machine-readable output like `login --print-token`.
	browser.Stdout = os.Stderr
}

type nonInteractiveContextKey struct{}
type outputWriterContextKey struct{}

// WithNonInteractive marks ctx as safe for GUI orchestration: token acquisition
// must not prompt or read from stdin.
func WithNonInteractive(ctx context.Context) context.Context {
	return context.WithValue(ctx, nonInteractiveContextKey{}, true)
}

// WithOutputWriter routes token-acquisition user messages to w. They default
// to stderr to keep stdout reserved for machine-readable output like
// `login --print-token`.
func WithOutputWriter(ctx context.Context, w io.Writer) context.Context {
	if w == nil {
		return ctx
	}
	return context.WithValue(ctx, outputWriterContextKey{}, w)
}

func isNonInteractive(ctx context.Context) bool {
	v, _ := ctx.Value(nonInteractiveContextKey{}).(bool)
	return v
}

func outputWriter(ctx context.Context) io.Writer {
	if w, _ := ctx.Value(outputWriterContextKey{}).(io.Writer); w != nil {
		return w
	}
	// User-facing auth prompts go to stderr so stdout stays clean for
	// piping (e.g. `obot login --print-token`).
	return os.Stderr
}

// AppURLForAPIBaseURL returns the app URL that owns credentials for an
// API base URL.
func AppURLForAPIBaseURL(baseURL string) (string, error) {
	appURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	appURL = strings.TrimSuffix(appURL, "/api")
	return localconfig.NormalizeAppURL(appURL)
}

// Logout removes the keyring token for appURL. It returns false when no
// token was stored for the URL.
func Logout(appURL string) (bool, error) {
	appURL, err := localconfig.NormalizeAppURL(appURL)
	if err != nil {
		return false, err
	}

	if _, err := credentialStore.Get(appURL); err != nil {
		if credentials.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, credentialStore.Delete(appURL)
}

// StoredTokenValid reports whether a stored token for appURL authenticates
// successfully. It never initiates login or prompts for user input.
func StoredTokenValid(ctx context.Context, appURL string) (bool, error) {
	appURL, err := localconfig.NormalizeAppURL(appURL)
	if err != nil {
		return false, err
	}

	token, err := credentialStore.Get(appURL)
	if err != nil {
		if credentials.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	return testToken(ctx, localconfig.APIBaseURL(appURL), token), nil
}

func ExistingToken(ctx context.Context, baseURL string) (string, error) {
	if testToken(ctx, baseURL, "") {
		return "", nil
	}

	appURL, err := AppURLForAPIBaseURL(baseURL)
	if err != nil {
		return "", err
	}

	token, err := credentialStore.Get(appURL)
	if err != nil {
		if credentials.IsNotFound(err) {
			return "", fmt.Errorf("no existing login for %s", appURL)
		}
		return "", err
	}
	if !testToken(ctx, baseURL, token) {
		return "", fmt.Errorf("stored login for %s is not valid", appURL)
	}
	return token, nil
}

func enter(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := fmt.Scanln()
		done <- err
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func Token(ctx context.Context, baseURL string, opts apiclient.TokenFetchOptions) (string, error) {
	// Check to see if authentication is required for this baseURL
	if testToken(ctx, baseURL, "") {
		return "", nil
	}

	appURL, err := AppURLForAPIBaseURL(baseURL)
	if err != nil {
		return "", err
	}

	token, tokenErr := credentialStore.Get(appURL)
	hasStoredToken := tokenErr == nil
	if tokenErr != nil && !credentials.IsNotFound(tokenErr) {
		return "", tokenErr
	}
	if hasStoredToken && !opts.ForceRefresh && testToken(ctx, baseURL, token) {
		return token, nil
	}

	authProviders, err := getAuthProviderServiceInfo(ctx, baseURL)
	if err != nil {
		return "", err
	} else if len(authProviders) == 0 {
		return "", fmt.Errorf("no auth providers found")
	}

	ctx, sigCancel := signal.NotifyContext(ctx, os.Interrupt)
	defer sigCancel()

	provider, err := userSelectAuthProvider(ctx, authProviders)
	if err != nil {
		return "", err
	}

	uuid := uuid.NewString()
	loginURL, err := create(ctx, baseURL, uuid, provider.ID, provider.Namespace, opts.Name, opts.Description, opts.NoExpiration, opts.Scopes)
	if err != nil {
		return "", fmt.Errorf("failed to create login request: %w", err)
	}

	if !hasStoredToken && !isNonInteractive(ctx) {
		w := outputWriter(ctx)
		fmt.Fprintln(w)
		fmt.Fprintln(w, color.GreenString("Authentication is needed"))
		fmt.Fprintln(w, color.GreenString("========================"))
		fmt.Fprintln(w)
		fmt.Fprintln(w, color.CyanString(provider.Name)+" is used for authentication using the browser. This can be bypassed by setting")
		fmt.Fprintln(w, "the env var "+color.CyanString("OBOT_API_KEY")+" to your API key.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, color.GreenString("Press ENTER to continue (CTRL+C to exit)"))
		if err := enter(ctx); err != nil {
			return "", err
		}
		fmt.Fprintln(w)
	}

	fmt.Fprintf(outputWriter(ctx), "Opening browser to %s. if there is an issue paste this link into a browser manually\n", loginURL)
	_ = openBrowser(loginURL)

	ctx, timeoutCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer timeoutCancel()

	token, err = get(ctx, baseURL, uuid)
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}

	return token, credentialStore.Set(appURL, token)
}

type createRequest struct {
	Name              string             `json:"name,omitempty"`
	Description       string             `json:"description,omitempty"`
	ProviderName      string             `json:"providerName,omitempty"`
	ProviderNamespace string             `json:"providerNamespace,omitempty"`
	ID                string             `json:"id,omitempty"`
	NoExpiration      bool               `json:"noExpiration,omitempty"`
	Scopes            types.APIKeyScopes `json:"scopes,omitempty"`
}

type createResponse struct {
	TokenPath string `json:"token-path,omitempty"`
}

func create(ctx context.Context, baseURL, uuid, providerName, providerNamespace, tokenName, tokenDescription string, noExpiration bool, scopes []string) (string, error) {
	apiScopes := types.APIKeyScopes{
		CanAccessSkills:   slices.Contains(scopes, "skills"),
		CanAccessAPI:      slices.Contains(scopes, "api"),
		CanAccessLLMProxy: slices.Contains(scopes, "llm"),
	}
	if slices.Contains(scopes, "all-mcp") {
		apiScopes.MCPServerIDs = []string{"*"}
	}
	var data bytes.Buffer
	if err := json.NewEncoder(&data).Encode(createRequest{
		Name:              tokenName,
		Description:       tokenDescription,
		ID:                uuid,
		ProviderName:      providerName,
		ProviderNamespace: providerNamespace,
		NoExpiration:      noExpiration,
		Scopes:            apiScopes,
	}); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/token-request", &data)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer req.Body.Close()

	var tokenResponse createResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return "", err
	}

	if tokenResponse.TokenPath == "" {
		return "", fmt.Errorf("no token found in response to %s", req.URL)
	}

	return tokenResponse.TokenPath, nil
}

func get(ctx context.Context, baseURL, uuid string) (string, error) {
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/token-request/"+uuid, nil)
		if err != nil {
			return "", err
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		var checkResponse types.TokenRequest
		if err := json.NewDecoder(resp.Body).Decode(&checkResponse); err != nil {
			return "", err
		}

		if checkResponse.Error != "" {
			return "", errors.New(checkResponse.Error)
		}

		if checkResponse.Token != "" {
			return checkResponse.Token, nil
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Millisecond * 500):
		}
	}
}

func testToken(ctx context.Context, baseURL, token string) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/me", nil)
	if err != nil {
		return false
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	var user types2.User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return false
	}

	return resp.StatusCode == 200 && user.Username != "anonymous"
}

func getAuthProviderServiceInfo(ctx context.Context, baseURL string) ([]types2.AuthProvider, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/auth-providers", nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var authProviders types2.AuthProviderList
	if err := json.NewDecoder(resp.Body).Decode(&authProviders); err != nil {
		return nil, err
	}

	if len(authProviders.Items) == 0 {
		return nil, fmt.Errorf("no auth providers found")
	}

	return authProviders.Items, nil
}

func userSelectAuthProvider(ctx context.Context, authProviders []types2.AuthProvider) (types2.AuthProvider, error) {
	var configuredAuthProviders []types2.AuthProvider
	for _, provider := range authProviders {
		if provider.Configured {
			configuredAuthProviders = append(configuredAuthProviders, provider)
		}
	}

	if len(configuredAuthProviders) == 0 {
		return types2.AuthProvider{}, fmt.Errorf("no configured auth providers found")
	} else if len(configuredAuthProviders) == 1 {
		return configuredAuthProviders[0], nil
	}
	if isNonInteractive(ctx) {
		return types2.AuthProvider{}, fmt.Errorf("multiple configured auth providers found; interactive provider selection is not available in non-interactive mode")
	}

	sort.Slice(configuredAuthProviders, func(i, j int) bool {
		return configuredAuthProviders[i].Name < configuredAuthProviders[j].Name
	})
	w := outputWriter(ctx)
	fmt.Fprintln(w)
	fmt.Fprintln(w, color.CyanString("Select an authentication provider:"))
	for i, provider := range configuredAuthProviders {
		fmt.Fprintf(w, "  %d. %s\n", i+1, provider.Name)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, color.GreenString("Enter the number of the provider you want to use:"))

	var choice int
	if _, err := fmt.Scanln(&choice); err != nil {
		return types2.AuthProvider{}, fmt.Errorf("error reading choice: %w", err)
	}

	if choice < 1 || choice > len(configuredAuthProviders) {
		return types2.AuthProvider{}, fmt.Errorf("invalid choice %d", choice)
	}

	return configuredAuthProviders[choice-1], nil
}
