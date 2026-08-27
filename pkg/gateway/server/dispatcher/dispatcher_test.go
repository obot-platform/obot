package dispatcher

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
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
	credentialEnvironment := map[string]string{"CLIENT_SECRET": "client-secret"}
	authProvider := acknowledgedAuthProvider(t, credentialEnvironment)
	gatewayClient := newDispatcherTestGatewayClient(t)
	if err := gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: authProvider.Name,
		Name:    authProvider.Name,
		Secrets: credentialEnvironment,
	}); err != nil {
		t.Fatal(err)
	}
	dispatcher := New(nil, fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(authProvider).Build(), gatewayClient, nil, "", "", "")
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
	key := providerKeyForAuthProvider(authProvider.Namespace, authProvider.Name)
	stopped := false
	dispatcher.ports.daemons[key] = daemonState{
		port:              12345,
		stop:              func() { stopped = true },
		command:           &exec.Cmd{},
		configurationHash: authProvider.Status.DaemonConfigurationHash,
	}

	_, err := dispatcher.URLForAuthProvider(t.Context(), authProvider.Namespace, authProvider.Name)
	if err == nil {
		t.Fatal("expected configuration updating error")
	}
	if !stopped {
		t.Fatal("stale daemon was not stopped")
	}
	if _, ok := dispatcher.daemonState(key); ok {
		t.Fatal("daemon was launched from an unacknowledged credential")
	}
}

func TestURLForAuthProviderRejectsDeletedCredentialWithCachedDaemon(t *testing.T) {
	authProvider := acknowledgedAuthProvider(t, map[string]string{"CLIENT_SECRET": "old-secret"})
	gatewayClient := newDispatcherTestGatewayClient(t)
	dispatcher := New(nil, fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(authProvider).Build(), gatewayClient, nil, "", "", "")
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
	if err == nil {
		t.Fatal("expected not configured error")
	}
	if !stopped {
		t.Fatal("stale daemon was not stopped")
	}
	if _, ok := dispatcher.daemonState(key); ok {
		t.Fatal("daemon using deleted credential was retained")
	}
}

func TestModelsForProviderReusesDaemon(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[]}`)
	}))
	t.Cleanup(server.Close)

	modelProvider := v1.ModelProvider{
		Name:      "model-provider",
		Namespace: "default",
	}
	dispatcher := New(nil, nil, nil, nil, "", "", "")
	t.Cleanup(dispatcher.Close)
	key := providerKeyForModelProvider(modelProvider.Namespace, modelProvider.Name)
	command := &exec.Cmd{}
	dispatcher.ports.daemons[key] = daemonState{
		port:    int64(server.Listener.Addr().(*net.TCPAddr).Port),
		stop:    func() {},
		command: command,
	}

	for range 2 {
		if _, err := dispatcher.ModelsForProvider(t.Context(), modelProvider); err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 2 {
		t.Fatalf("model requests = %d, want 2", requests.Load())
	}
	state, ok := dispatcher.daemonState(key)
	if !ok || state.command != command {
		t.Fatal("cached model provider daemon was replaced")
	}
}

func TestModelsForProviderUsesReconciledObject(t *testing.T) {
	modelProvider := &v1.ModelProvider{
		Name:      "model-provider",
		Namespace: "default",
		Status: v1.ModelProviderStatus{
			MissingConfigurationParameters: []string{"RECONCILED_PARAMETER"},
		},
	}
	persistedModelProvider := modelProvider.DeepCopy()
	persistedModelProvider.Status = v1.ModelProviderStatus{
		MissingConfigurationParameters: []string{"PERSISTED_PARAMETER"},
	}
	dispatcher := New(nil, fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(persistedModelProvider).Build(), nil, nil, "", "", "")
	t.Cleanup(dispatcher.Close)

	_, err := dispatcher.ModelsForProvider(t.Context(), *modelProvider)
	if err == nil || !strings.Contains(err.Error(), "RECONCILED_PARAMETER") {
		t.Fatalf("ModelsForProvider() error = %v, want reconciled object error", err)
	}
	if strings.Contains(err.Error(), "PERSISTED_PARAMETER") {
		t.Fatalf("ModelsForProvider() used persisted object: %v", err)
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
