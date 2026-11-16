//go:build integration

// Package billing runs Obot with billing on against a fake obot-billing, and checks what the subscription's
// entitlements allow and refuse. Nothing is deployed: limits apply when an object is written.
package billing

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
	billingKey    = "integration-billing-key"
	storageToken  = "integration-storage-token"
	setupToken    = "integration-setup-token-0123456789abcdef0123456789abcdef"
	ownerEmail    = "owner@integration.example.com"
	ownerPassword = "Integration-Owner-Passw0rd!"
)

var (
	// billing is the fake obot-billing the app syncs with.
	billing *fakeBilling
	// storage reads and writes the app's storage directly, to set up what would otherwise take days.
	storage kclient.Client
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	docker := startFakeDocker()
	defer docker.Close()
	if err := os.Setenv("DOCKER_HOST", docker.host()); err != nil {
		fmt.Fprintf(os.Stderr, "set DOCKER_HOST: %v\n", err)
		return 1
	}

	billing = startFakeBilling(billingKey)
	defer billing.server.Close()

	app, err := harness.StartApp(func(config *services.Config) {
		config.BillingURL = billing.server.URL
		config.BillingKey = billingKey
		config.StorageToken = storageToken
		config.EnableAuthentication = true
		config.LocalAuthInitialOwnerEmail = ownerEmail
		config.LocalAuthInitialOwnerSetupToken = setupToken
		config.LocalAuthInitialOwnerSetupTokenExpirationHours = 1
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "start Obot with billing: %v\n", err)
		return 1
	}

	code := 1
	if err := setUp(app); err != nil {
		fmt.Fprintf(os.Stderr, "set up Obot with billing: %v\n", err)
	} else {
		code = m.Run()
	}
	if err := app.Stop(); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "stop Obot with billing: %v\n", err)
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
