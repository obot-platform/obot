//go:build integration

// Package agentdelegation runs Obot with authentication on, so that several users can sign in, and checks end to end
// that an agent acts on behalf of a user only after that user approved it in the browser session the agent asked
// from. The MCP server the agent reaches is a remote one this test serves, so nothing needs Docker or NPX.
package agentdelegation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/services"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/tests/integration/harness"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// These are test values for the app this suite starts.
const (
	storageToken   = "integration-storage-token"
	setupToken     = "integration-setup-token-0123456789abcdef0123456789abcdef"
	ownerEmail     = "owner@integration.example.com"
	ownerPassword  = "Integration-Owner-Passw0rd!"
	memberPassword = "Integration-Member-Passw0rd!"
)

var (
	// storage reads the app's storage directly, to see what no API shows: the setup a user did through an agent.
	storage kclient.Client
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	docker := harness.StartFakeDocker()
	defer docker.Close()
	if err := os.Setenv("DOCKER_HOST", docker.Host()); err != nil {
		fmt.Fprintf(os.Stderr, "set DOCKER_HOST: %v\n", err)
		return 1
	}

	app, err := harness.StartApp(func(config *services.Config) {
		config.StorageToken = storageToken
		config.EnableAuthentication = true
		config.LocalAuthInitialOwnerEmail = ownerEmail
		config.LocalAuthInitialOwnerSetupToken = setupToken
		config.LocalAuthInitialOwnerSetupTokenExpirationHours = 1
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "start Obot with authentication: %v\n", err)
		return 1
	}

	code := 1
	if err := setUp(app); err != nil {
		fmt.Fprintf(os.Stderr, "set up Obot with authentication: %v\n", err)
	} else {
		code = m.Run()
	}
	if err := app.Stop(); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "stop Obot: %v\n", err)
		code = 1
	}
	return code
}

func setUp(app *harness.App) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := harness.ActivateOwner(ctx, app.BaseURL, setupToken, ownerPassword); err != nil {
		return fmt.Errorf("activate the owner: %w", err)
	}

	var err error
	storage, err = kclient.New(&rest.Config{
		Host:        fmt.Sprintf("https://127.0.0.1:%d", app.StoragePort),
		BearerToken: storageToken,
		Insecure:    true,
	}, kclient.Options{Scheme: scheme.Scheme})
	if err != nil {
		return fmt.Errorf("connect to storage: %w", err)
	}
	return nil
}
