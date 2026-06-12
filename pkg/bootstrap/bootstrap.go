package bootstrap

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	ObotBootstrapCookie = "obot-bootstrap"
	bootstrapUsername   = "bootstrap"
)

type Bootstrap struct {
	token, serverURL                  string
	authEnabled, forceEnableBootstrap bool
	gatewayClient                     *client.Client
	authProviderGetter                configuredAuthProviderGetter
}

type configuredAuthProviderGetter interface {
	GetConfiguredAuthProvider(context.Context) (string, error)
}

func New(ctx context.Context, serverURL string, c *client.Client, authProviderGetter configuredAuthProviderGetter, authEnabled, forceEnableBootstrap bool) (*Bootstrap, error) {
	if !authEnabled {
		// Auth is not enabled, so skip token generation.
		return &Bootstrap{
			serverURL:            serverURL,
			authEnabled:          authEnabled,
			forceEnableBootstrap: forceEnableBootstrap,
			gatewayClient:        c,
			authProviderGetter:   authProviderGetter,
		}, nil
	}

	token := os.Getenv("OBOT_BOOTSTRAP_TOKEN")
	tokenFromCredential, exists, err := getTokenFromCredential(ctx, c)
	if err != nil {
		return nil, err
	}

	if token != "" && !exists {
		// Save the token from the env var to the credential.
		if err := saveTokenToCredential(ctx, token, c); err != nil {
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

			if err := saveTokenToCredential(ctx, token, c); err != nil {
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
		authProviderGetter:   authProviderGetter,
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

func getTokenFromCredential(ctx context.Context, c *client.Client) (string, bool, error) {
	tokenCredential, err := c.RevealCredential(ctx, []string{ObotBootstrapCookie}, ObotBootstrapCookie)
	if err != nil {
		if errors.As(err, &client.CredentialNotFoundError{}) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get bootstrap token credential: %w", err)
	}

	value, ok := tokenCredential.Secrets["token"]
	if !ok {
		return "", false, nil
	}
	return value, true, nil
}

func saveTokenToCredential(ctx context.Context, token string, c *client.Client) error {
	credential := types.Credential{
		Name:    ObotBootstrapCookie,
		Context: ObotBootstrapCookie,
		Secrets: map[string]string{
			"token": token,
		},
	}

	if err := c.UpsertCredential(ctx, credential); err != nil {
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
		c, err := req.Cookie(ObotBootstrapCookie)
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
			ProviderUsername:     "bootstrap",
			ProviderUserID:       "bootstrap",
			HashedProviderUserID: hash.String("bootstrap"),
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
			Groups: []string{types2.GroupOwner, types2.GroupAdmin, types2.GroupBasic, types2.GroupAuthenticated},
			Extra: map[string][]string{
				"auth_provider_name": {"bootstrap"},
			},
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
			fmt.Printf("WARNING: bootstrap login failed: failed to check if bootstrap is enabled: %v\n", err)
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
		Name:     ObotBootstrapCookie,
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
		Name:     ObotBootstrapCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.HasPrefix(b.serverURL, "https://"),
	})

	return nil
}

func (b *Bootstrap) IsEnabled(req api.Context) error {
	setupEnabled, err := b.SetupEnabled(req.Context())
	if err != nil {
		return err
	}

	bootstrapEnabled := setupEnabled || (b.authEnabled && b.forceEnableBootstrap)

	return req.Write(map[string]bool{
		"enabled":      bootstrapEnabled,
		"setupEnabled": setupEnabled,
	})
}

func (b *Bootstrap) Enabled(ctx context.Context) (bool, error) {
	if !b.authEnabled {
		return false, nil
	}

	return b.bootstrapEnabled(ctx)
}

func (b *Bootstrap) SetupEnabled(ctx context.Context) (bool, error) {
	if !b.authEnabled {
		return false, nil
	}

	return b.setupEnabled(ctx)
}

// bootstrapEnabled determines whether the bootstrap user is currently available for login.
func (b *Bootstrap) bootstrapEnabled(ctx context.Context) (bool, error) {
	if b.forceEnableBootstrap {
		return true, nil
	}

	return b.setupEnabled(ctx)
}

// setupEnabled determines whether bootstrap setup flow is currently available.
// It is available while there is no configured auth provider, or until an owner
// user exists from the currently configured auth provider.
func (b *Bootstrap) setupEnabled(ctx context.Context) (bool, error) {
	if b.authProviderGetter == nil {
		return false, errors.New("configured auth provider getter is not set")
	}

	configuredAuthProvider, err := b.authProviderGetter.GetConfiguredAuthProvider(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	if configuredAuthProvider == "" {
		return true, nil
	}

	ownerUsers, err := b.gatewayClient.Users(ctx, types.UserQuery{
		Role: types2.RoleOwner,
	})
	if err != nil {
		return false, fmt.Errorf("failed to get owner users: %w", err)
	}

	for _, u := range ownerUsers {
		if u.Username == bootstrapUsername || u.Email == "" {
			continue
		}

		hasIdentity, err := b.gatewayClient.UserHasIdentityForAuthProvider(ctx, u.ID, configuredAuthProvider)
		if err != nil {
			return false, fmt.Errorf("failed to check owner auth provider identity: %w", err)
		}
		if hasIdentity {
			return false, nil
		}
	}

	return true, nil
}
