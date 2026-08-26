package dispatcher

import (
	"log/slog"
	"os/exec"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	authproviderconfig "github.com/obot-platform/obot/pkg/authprovider"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestProviderLogLevelEnv(t *testing.T) {
	originalLevel := logger.Level()
	t.Cleanup(func() {
		logger.SetLevel(originalLevel)
	})

	t.Run("defaults to info", func(t *testing.T) {
		logger.SetLevel(slog.LevelInfo)

		if got := providerLogLevel(); got != "INFO" {
			t.Fatalf("providerLogLevel() = %q, want INFO", got)
		}
	})

	t.Run("uses debug when logger is debug", func(t *testing.T) {
		logger.SetDebug()

		if got := providerLogLevel(); got != "DEBUG" {
			t.Fatalf("providerLogLevel() = %q, want DEBUG", got)
		}
	})
}

func TestURLForAuthProviderReturnsMatchingAcknowledgedDaemon(t *testing.T) {
	authProvider := acknowledgedAuthProvider(t, map[string]string{"CLIENT_SECRET": "client-secret"})
	dispatcher := New(nil, fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(authProvider).Build(), nil, nil, "", "", "")
	t.Cleanup(dispatcher.Close)
	key := providerKeyForAuthProvider(authProvider.Namespace, authProvider.Name)
	dispatcher.ports.daemons[key] = daemonState{
		port:              12345,
		stop:              func() {},
		command:           &exec.Cmd{},
		configurationHash: authProvider.Status.DaemonConfigurationHash,
	}

	u, err := dispatcher.URLForAuthProvider(t.Context(), authProvider.Namespace, authProvider.Name)
	if err != nil {
		t.Fatal(err)
	}
	if u.String() != "http://127.0.0.1:12345" {
		t.Fatalf("URL = %q, want http://127.0.0.1:12345", u.String())
	}
}

func TestURLForAuthProviderStopsDaemonForUnacknowledgedRevision(t *testing.T) {
	authProvider := acknowledgedAuthProvider(t, map[string]string{"CLIENT_SECRET": "client-secret"})
	authProvider.Annotations[v1.AuthProviderSyncAnnotation] = "new-revision"
	dispatcher := New(nil, fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(authProvider).Build(), nil, nil, "", "", "")
	t.Cleanup(dispatcher.Close)
	key := providerKeyForAuthProvider(authProvider.Namespace, authProvider.Name)
	stopped := false
	dispatcher.ports.daemons[key] = daemonState{
		port:              12345,
		stop:              func() { stopped = true },
		command:           &exec.Cmd{},
		configurationHash: authProvider.Status.DaemonConfigurationHash,
	}

	_, err := dispatcher.URLForAuthProvider(t.Context(), authProvider.Namespace, authProvider.Name)
	if err == nil || !stopped {
		t.Fatalf("URLForAuthProvider() error = %v, stopped = %v; want updating error and stopped daemon", err, stopped)
	}
	if _, ok := dispatcher.daemonState(key); ok {
		t.Fatal("stale daemon state was retained")
	}
}

func TestURLForAuthProviderRejectsUnacknowledgedCredentialChange(t *testing.T) {
	oldCredential := map[string]string{"CLIENT_SECRET": "old-secret"}
	authProvider := acknowledgedAuthProvider(t, oldCredential)
	gatewayClient := newDispatcherTestGatewayClient(t)
	if err := gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: authProvider.Name,
		Name:    authProvider.Name,
		Secrets: map[string]string{"CLIENT_SECRET": "new-secret"},
	}); err != nil {
		t.Fatal(err)
	}
	dispatcher := New(nil, fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(authProvider).Build(), gatewayClient, nil, "", "", "")
	t.Cleanup(dispatcher.Close)

	_, err := dispatcher.URLForAuthProvider(t.Context(), authProvider.Namespace, authProvider.Name)
	if err == nil {
		t.Fatal("expected configuration updating error")
	}
	if _, ok := dispatcher.daemonState(providerKeyForAuthProvider(authProvider.Namespace, authProvider.Name)); ok {
		t.Fatal("daemon was launched from an unacknowledged credential")
	}
}

func TestStopDaemonCommandDoesNotRemoveReplacement(t *testing.T) {
	dispatcher := New(nil, nil, nil, nil, "", "", "")
	t.Cleanup(dispatcher.Close)
	oldCommand := &exec.Cmd{}
	replacementCommand := &exec.Cmd{}
	replacementStopped := false
	dispatcher.ports.daemons["provider"] = daemonState{
		port:    12345,
		stop:    func() { replacementStopped = true },
		command: replacementCommand,
	}

	dispatcher.stopDaemonCommand("provider", oldCommand)
	state, ok := dispatcher.daemonState("provider")
	if !ok || state.command != replacementCommand {
		t.Fatal("old command removed its replacement")
	}
	if replacementStopped {
		t.Fatal("old command stopped its replacement")
	}
}

func TestKeyedMutexDoesNotBlockAnotherProvider(t *testing.T) {
	locks := newKeyedMutex()
	unlockSlow := locks.Lock("slow-provider")
	defer unlockSlow()

	acquired := make(chan struct{})
	go func() {
		unlockFast := locks.Lock("fast-provider")
		close(acquired)
		unlockFast()
	}()

	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("slow provider blocked another provider")
	}
}

func acknowledgedAuthProvider(t *testing.T, credentialEnvironment map[string]string) *v1.AuthProvider {
	t.Helper()
	authProvider := &v1.AuthProvider{
		Name:       "entra-auth-provider",
		Namespace:  "default",
		UID:        types.UID("provider-uid"),
		Generation: 2,
		Annotations: map[string]string{
			v1.AuthProviderSyncAnnotation: "revision-one",
		},
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				CommonProviderMetadata: clienttypes.CommonProviderMetadata{
					Command: "provider-command",
					RequiredConfigurationParameters: []clienttypes.ProviderConfigurationParameter{
						{
							Name: "CLIENT_SECRET",
						},
					},
				},
			},
		},
	}
	hash, err := authproviderconfig.DaemonConfigurationHash(*authProvider, credentialEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	authProvider.Status = v1.AuthProviderStatus{
		Configured:              true,
		ObservedGeneration:      authProvider.Generation,
		DaemonConfigurationHash: hash,
		ObservedSyncRevision:    authProvider.Annotations[v1.AuthProviderSyncAnnotation],
	}
	return authProvider
}

func newDispatcherTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	gatewayClient := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() {
		if err := gatewayClient.Close(); err != nil {
			t.Errorf("close gateway client: %v", err)
		}
	})
	return gatewayClient
}
