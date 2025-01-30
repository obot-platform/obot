package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"time"

	"github.com/adrg/xdg"
	"github.com/fatih/color"
	"github.com/google/uuid"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/pkg/browser"
)

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

func Token(ctx context.Context, baseURL string) (string, error) {
	// Check to see if authentication is required for this baseURL
	if testToken(ctx, baseURL, "") {
		return "", nil
	}

	authProviders, err := getAuthProviderServiceInfo(ctx, baseURL)
	if err != nil {
		return "", err
	} else if len(authProviders) == 0 {
		return "", fmt.Errorf("no auth providers found")
	}

	ctx, sigCancel := signal.NotifyContext(ctx, os.Interrupt)
	defer sigCancel()

	tokenFile, err := xdg.ConfigFile(filepath.Join("obot", "token"))
	if err != nil {
		return "", err
	}

	var existed bool
	tokenData, err := os.ReadFile(tokenFile)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("reading %s: %w", tokenFile, err)
	} else if err == nil {
		existed = true
	}

	var scopedTokens map[string]string
	if err = json.Unmarshal(tokenData, &scopedTokens); err != nil {
		// Ignore unmarshal errors and just store new tokens.
		scopedTokens = make(map[string]string, 1)
	}

	if token, ok := scopedTokens[baseURL]; ok && testToken(ctx, baseURL, token) {
		return token, nil
	}

	provider, err := userSelectAuthProvider(authProviders)
	if err != nil {
		return "", err
	}

	uuid := uuid.NewString()
	loginURL, err := create(ctx, baseURL, uuid, provider.ID, provider.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to create login request: %w", err)
	}

	if !existed {
		fmt.Println()
		fmt.Println(color.GreenString("Authentication is needed"))
		fmt.Println(color.GreenString("========================"))
		fmt.Println()
		fmt.Println(color.CyanString(provider.Name) + " is used for authentication using the browser. This can be bypassed by setting")
		fmt.Println("the env var " + color.CyanString("OBOT_API_KEY") + " to your API key.")
		fmt.Println()
		fmt.Println(color.GreenString("Press ENTER to continue (CTRL+C to exit)"))
		if err := enter(ctx); err != nil {
			return "", err
		}
		fmt.Println()
	}

	fmt.Printf("Opening browser to %s. if there is an issue paste this link into a browser manually\n", loginURL)
	_ = browser.OpenURL(loginURL)

	ctx, timeoutCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer timeoutCancel()

	token, err := get(ctx, baseURL, uuid)
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}

	scopedTokens[baseURL] = token
	tokenData, err = json.Marshal(scopedTokens)
	if err != nil {
		return "", fmt.Errorf("failed to store token: %w", err)
	}

	return token, os.WriteFile(tokenFile, tokenData, 0600)
}

type createRequest struct {
	ProviderName      string `json:"providerName,omitempty"`
	ProviderNamespace string `json:"providerNamespace,omitempty"`
	ID                string `json:"id,omitempty"`
}

type createResponse struct {
	TokenPath string `json:"token-path,omitempty"`
}

func create(ctx context.Context, baseURL, uuid, providerName, providerNamespace string) (string, error) {
	var data bytes.Buffer
	if err := json.NewEncoder(&data).Encode(createRequest{
		ID:                uuid,
		ProviderName:      providerName,
		ProviderNamespace: providerNamespace,
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
	req.Header.Set("Authorization", "Bearer "+token)

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

func userSelectAuthProvider(authProviders []types2.AuthProvider) (types2.AuthProvider, error) {
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

	sort.Slice(configuredAuthProviders, func(i, j int) bool {
		return configuredAuthProviders[i].Name < configuredAuthProviders[j].Name
	})
	fmt.Println()
	fmt.Println(color.CyanString("Select an authentication provider:"))
	for i, provider := range configuredAuthProviders {
		fmt.Printf("  %d. %s\n", i+1, provider.Name)
	}
	fmt.Println()
	fmt.Println(color.GreenString("Enter the number of the provider you want to use:"))

	var choice int
	if _, err := fmt.Scanln(&choice); err != nil {
		return types2.AuthProvider{}, fmt.Errorf("error reading choice: %w", err)
	}

	if choice < 1 || choice > len(configuredAuthProviders) {
		return types2.AuthProvider{}, fmt.Errorf("invalid choice %d", choice)
	}

	return configuredAuthProviders[choice-1], nil
}
