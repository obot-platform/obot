package bootstrap

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	obotBootstrap = "obot-bootstrap"
)

type Bootstrap struct {
	token, serverURL                  string
	authEnabled, forceEnableBootstrap bool
	gatewayClient                     *client.Client
}

func New(ctx context.Context, serverURL string, c *client.Client, g *gptscript.GPTScript, authEnabled, forceEnableBootstrap bool) (*Bootstrap, error) {
	if !authEnabled {
		// Auth is not enabled, so skip token generation.
		return &Bootstrap{
			serverURL:            serverURL,
			authEnabled:          authEnabled,
			forceEnableBootstrap: forceEnableBootstrap,
			gatewayClient:        c,
		}, nil
	}

	token := os.Getenv("OBOT_BOOTSTRAP_TOKEN")
	tokenFromCredential, exists, err := getTokenFromCredential(ctx, g)
	if err != nil {
		return nil, err
	}

	if token != "" && !exists {
		// Save the token from the env var to the credential.
		if err := saveTokenToCredential(ctx, token, g); err != nil {
			return nil, err
		}
	} else if token == "" {
		if exists {
			// Just use the token from the credential, since it already exists.
			token = tokenFromCredential
		} else {
			// Generate a new token, save it in the credential, and print it to the logs.
			bytes := make([]byte, 32)
			_, err := rand.Read(bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to generate random token: %w", err)
			}

			token = fmt.Sprintf("%x", bytes)

			if err := saveTokenToCredential(ctx, token, g); err != nil {
				return nil, err
			}
		}
	}

	if len(token) < 6 {
		return nil, errors.New("error: bootstrap token must be at least 6 characters")
	}

	b := &Bootstrap{
		token:                token,
		authEnabled:          authEnabled,
		serverURL:            serverURL,
		forceEnableBootstrap: forceEnableBootstrap,
		gatewayClient:        c,
	}

	bootstrapEnabled, err := b.bootstrapEnabled(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check if bootstrap is enabled: %w", err)
	}
	if bootstrapEnabled {
		printToken(token)
	}

	return b, nil
}

func getTokenFromCredential(ctx context.Context, g *gptscript.GPTScript) (string, bool, error) {
	tokenCredential, err := g.RevealCredential(ctx, []string{obotBootstrap}, obotBootstrap)
	if err != nil {
		if errors.As(err, &gptscript.ErrNotFound{}) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get bootstrap token credential: %w", err)
	}

	value, ok := tokenCredential.Env["token"]
	if !ok {
		return "", false, nil
	}
	return value, true, nil
}

func saveTokenToCredential(ctx context.Context, token string, g *gptscript.GPTScript) error {
	credential := gptscript.Credential{
		ToolName: obotBootstrap,
		Context:  obotBootstrap,
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			"token": token,
		},
	}

	if err := g.CreateCredential(ctx, credential); err != nil {
		return fmt.Errorf("failed to store bootstrap token credential: %w", err)
	}
	return nil
}

func printToken(token string) {
	message := "Bootstrap Token: " + token
	line := strings.Repeat("-", len(message)+4)

	fmt.Println(line)
	fmt.Println("| " + message + " |")
	fmt.Println(line)
}

func (b *Bootstrap) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	if !b.authEnabled {
		return nil, false, nil
	}

	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		// Check for the cookie.
		c, err := req.Cookie(obotBootstrap)
		if err != nil || c.Value != b.token {
			return nil, false, nil
		}
	} else if authHeader != fmt.Sprintf("Bearer %s", b.token) {
		return nil, false, nil
	}

	// Deny authentication if bootstrap is not enabled.
	if enabled, err := b.bootstrapEnabled(req.Context()); !enabled || err != nil {
		return nil, false, err
	}

	gatewayUser, err := b.gatewayClient.EnsureIdentityWithRole(
		req.Context(),
		&types.Identity{
			ProviderUsername: "bootstrap",
			ProviderUserID:   "bootstrap",
		},
		req.Header.Get("X-Obot-User-Timezone"),
		types2.RoleOwner,
	)
	if err != nil {
		return nil, false, err
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   "bootstrap",
			UID:    fmt.Sprintf("%d", gatewayUser.ID),
			Groups: []string{types2.GroupOwner, types2.GroupAdmin, types2.GroupAuthenticated},
		},
	}, true, nil
}

func (b *Bootstrap) Login(req api.Context) error {
	if !b.authEnabled {
		http.Error(req.ResponseWriter, "auth is not enabled", http.StatusNotFound)
		return nil
	}

	// Deny login attempts if bootstrap is not enabled.
	if enabled, err := b.bootstrapEnabled(req.Context()); !enabled || err != nil {
		http.Error(req.ResponseWriter, "invalid token", http.StatusUnauthorized)

		if err != nil {
			fmt.Printf("WARNING: bootstrap login failed: failed to check if admin user exists: %v\n", err)
		}
		return nil
	}

	auth := req.Request.Header.Get("Authorization")
	if auth == "" {
		http.Error(req.ResponseWriter, "missing Authorization header", http.StatusBadRequest)
		return nil
	} else if auth != fmt.Sprintf("Bearer %s", b.token) {
		http.Error(req.ResponseWriter, "invalid token", http.StatusUnauthorized)
		return nil
	}

	http.SetCookie(req.ResponseWriter, &http.Cookie{
		Name:     obotBootstrap,
		Value:    strings.TrimPrefix(auth, "Bearer "),
		Path:     "/",
		MaxAge:   60 * 60 * 24 * 7, // 1 week
		HttpOnly: true,
		Secure:   strings.HasPrefix(b.serverURL, "https://"),
	})
	http.Redirect(req.ResponseWriter, req.Request, "/admin/auth-providers", http.StatusFound)

	return nil
}

func (b *Bootstrap) Logout(req api.Context) error {
	http.SetCookie(req.ResponseWriter, &http.Cookie{
		Name:     obotBootstrap,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.HasPrefix(b.serverURL, "https://"),
	})

	return nil
}

func (b *Bootstrap) IsEnabled(req api.Context) error {
	if !b.authEnabled {
		return req.Write(map[string]bool{"enabled": false})
	}

	bootstrapEnabled, err := b.bootstrapEnabled(req.Context())
	if err != nil {
		return err
	}

	return req.Write(map[string]bool{"enabled": bootstrapEnabled})
}

func (b *Bootstrap) bootstrapEnabled(ctx context.Context) (bool, error) {
	if b.forceEnableBootstrap {
		return true, nil
	}

	adminUsers, err := b.gatewayClient.Users(ctx, types.UserQuery{
		Role: types2.RoleOwner,
	})
	if err != nil {
		return false, fmt.Errorf("failed to get admin users: %w", err)
	}

	for _, u := range adminUsers {
		if u.Username != "bootstrap" && u.Email != "" {
			// A non-bootstrap admin user exists, so bootstrap is not enabled
			return false, nil
		}
	}
	return true, nil
}
