package bootstrap

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const bootstrapCookie = "obot-bootstrap"

type Bootstrap struct {
	enableBootstrapUser bool
	token, serverURL    string
	dispatcher          *dispatcher.Dispatcher
	gatewayClient       *client.Client
}

func New(ctx context.Context, enableBootstrapUser bool, serverURL string, c *client.Client, d *dispatcher.Dispatcher) (*Bootstrap, error) {
	token := os.Getenv("OBOT_BOOTSTRAP_TOKEN")

	if token == "" && enableBootstrapUser {
		bytes := make([]byte, 32)
		_, err := rand.Read(bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to generate random token: %w", err)
		}

		token = fmt.Sprintf("%x", bytes)

		// We deliberately only print the token if it was not provided by the user.
		fmt.Printf("Bootstrap token: %s\nUse this token to log in to the Admin UI.\n", token)
	} else if !enableBootstrapUser {
		configuredAuthProviders, err := d.ListConfiguredAuthProviders(ctx, system.DefaultNamespace)
		if err == nil && len(configuredAuthProviders) == 0 {
			fmt.Printf("WARNING: Bootstrap user is disabled, and no auth providers are configured. You will be unable to log in to Obot.\n")
		}
	}

	return &Bootstrap{
		enableBootstrapUser: enableBootstrapUser,
		token:               token,
		serverURL:           serverURL,
		dispatcher:          d,
		gatewayClient:       c,
	}, nil
}

func (b *Bootstrap) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	if !b.enableBootstrapUser {
		return nil, false, nil
	}

	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		// Check for the cookie.
		c, err := req.Cookie(bootstrapCookie)
		if err != nil || c.Value != b.token {
			return nil, false, nil
		}
	} else if authHeader != fmt.Sprintf("Bearer %s", b.token) {
		return nil, false, nil
	}

	gatewayUser, err := b.gatewayClient.EnsureIdentityWithRole(
		req.Context(),
		&types.Identity{
			ProviderUsername: "bootstrap",
		},
		req.Header.Get("X-Obot-User-Timezone"),
		types2.RoleAdmin,
	)
	if err != nil {
		return nil, false, err
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   "bootstrap",
			UID:    fmt.Sprintf("%d", gatewayUser.ID),
			Groups: []string{authz.AdminGroup, authz.AuthenticatedGroup},
		},
	}, true, nil
}

func (b *Bootstrap) Login(req api.Context) error {
	if !b.enableBootstrapUser {
		http.Error(req.ResponseWriter, "invalid token", http.StatusUnauthorized)
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
		Name:     bootstrapCookie,
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
	fmt.Printf("logging out bootstrap user\n")
	http.SetCookie(req.ResponseWriter, &http.Cookie{
		Name:     bootstrapCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.HasPrefix(b.serverURL, "https://"),
	})

	return nil
}
