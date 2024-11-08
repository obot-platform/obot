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
	"strings"
	"time"

	"github.com/adrg/xdg"
	"github.com/fatih/color"
	"github.com/google/uuid"
	types2 "github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/gateway/types"
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

func Token(ctx context.Context, baseURL, appName string) (string, error) {
	// Check to see if authentication is required for this baseURL
	if testToken(ctx, baseURL, "") {
		return "", nil
	}

	serviceName, err := getAuthProviderServiceName(ctx, baseURL)
	if err != nil {
		return "", err
	}

	ctx, sigCancel := signal.NotifyContext(ctx, os.Interrupt)
	defer sigCancel()

	tokenFile, err := xdg.ConfigFile(filepath.Join(appName, "token"))
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

	token := strings.TrimSpace(string(tokenData))
	if testToken(ctx, baseURL, token) {
		return token, nil
	}

	uuid := uuid.NewString()
	loginURL, err := create(ctx, baseURL, uuid, serviceName)
	if err != nil {
		return "", fmt.Errorf("failed to create login request: %w", err)
	}

	if !existed {
		fmt.Println()
		fmt.Println(color.GreenString("Authentication is needed"))
		fmt.Println(color.GreenString("========================"))
		fmt.Println()
		fmt.Println(color.CyanString(serviceName) + " is used for authentication using the browser. This can be bypassed by setting")
		fmt.Println("the env var " + color.CyanString("OTTO_API_KEY") + " to your API key.")
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

	token, err = get(ctx, baseURL, uuid)
	if err != nil {
		return "", fmt.Errorf("failed to get token: %w", err)
	}

	return token, os.WriteFile(tokenFile, []byte(token), 0600)
}

type createRequest struct {
	ServiceName string `json:"serviceName,omitempty"`
	ID          string `json:"id,omitempty"`
}

type createResponse struct {
	TokenPath string `json:"token-path,omitempty"`
}

func create(ctx context.Context, baseURL, uuid, serviceName string) (string, error) {
	var data bytes.Buffer
	if err := json.NewEncoder(&data).Encode(createRequest{ID: uuid, ServiceName: serviceName}); err != nil {
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

func getAuthProviderServiceName(ctx context.Context, baseURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/auth-providers", nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var authProviders []types.AuthProvider
	if err := json.NewDecoder(resp.Body).Decode(&authProviders); err != nil {
		return "", err
	}

	if len(authProviders) == 0 {
		return "", fmt.Errorf("no auth providers found")
	}

	// Take the last auth provider. That is the one created most recently.
	return authProviders[len(authProviders)-1].ServiceName, nil
}
