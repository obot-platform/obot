package dispatcher

import (
	"errors"
	"log/slog"
	"testing"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
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

func TestAuthProviderRevisionTracksOnlyDaemonConfiguration(t *testing.T) {
	authProvider := v1.AuthProvider{
		Name:       "entra",
		Namespace:  "default",
		Generation: 4,
		Annotations: map[string]string{
			v1.AuthProviderSyncAnnotation: "revision-one",
		},
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				CommonProviderMetadata: clienttypes.CommonProviderMetadata{
					Command: "entra-auth-provider",
				},
			},
		},
	}

	original, err := authProviderRevision(authProvider)
	if err != nil {
		t.Fatal(err)
	}

	statusUpdate := authProvider.DeepCopy()
	statusUpdate.Status.Configured = true
	statusRevision, err := authProviderRevision(*statusUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if statusRevision != original {
		t.Fatalf("status update changed daemon revision: got %#v, want %#v", statusRevision, original)
	}

	metadataUpdate := authProvider.DeepCopy()
	metadataUpdate.Generation++
	metadataUpdate.Annotations["unrelated"] = "value"
	metadataRevision, err := authProviderRevision(*metadataUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if !metadataRevision.sameConfiguration(original) {
		t.Fatal("unrelated metadata update changed daemon configuration revision")
	}

	credentialUpdate := authProvider.DeepCopy()
	credentialUpdate.Generation++
	credentialUpdate.Annotations[v1.AuthProviderSyncAnnotation] = "revision-two"
	credentialRevision, err := authProviderRevision(*credentialUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if credentialRevision == original {
		t.Fatal("credential sync revision did not change daemon revision")
	}

	specUpdate := authProvider.DeepCopy()
	specUpdate.Generation++
	specUpdate.Spec.Command = "new-entra-auth-provider"
	specRevision, err := authProviderRevision(*specUpdate)
	if err != nil {
		t.Fatal(err)
	}
	if specRevision == original {
		t.Fatal("provider spec did not change daemon revision")
	}
}

func TestObserveDaemonRevisionStopsOnlyStaleDaemon(t *testing.T) {
	d := &Dispatcher{ports: newPorts()}
	t.Cleanup(d.ports.daemonClose)
	key := providerKeyForAuthProvider("default", "entra")
	oldRevision := daemonRevision{
		generation: 1,
		value:      "old",
	}
	newRevision := daemonRevision{
		generation: 2,
		value:      "new",
	}
	stopCalls := 0
	d.ports.daemonPorts[key] = 10443
	d.ports.daemonsRunning[key] = func() {
		stopCalls++
	}
	d.ports.daemonRevisions[key] = oldRevision

	if current := d.observeDaemonRevision(key, oldRevision); !current {
		t.Fatal("current revision was rejected")
	}
	if stopCalls != 0 {
		t.Fatalf("matching daemon stop calls = %d, want 0", stopCalls)
	}

	if current := d.observeDaemonRevision(key, newRevision); !current {
		t.Fatal("new revision was rejected")
	}
	if stopCalls != 1 {
		t.Fatalf("stale daemon stop calls = %d, want 1", stopCalls)
	}
	if _, running := d.ports.daemonsRunning[key]; running {
		t.Fatal("stale daemon remains in registry")
	}

	d.ports.daemonPorts[key] = 10444
	d.ports.daemonsRunning[key] = func() {
		stopCalls++
	}
	d.ports.daemonRevisions[key] = newRevision
	if current := d.observeDaemonRevision(key, oldRevision); current {
		t.Fatal("older revision superseded newer desired revision")
	}
	if stopCalls != 1 {
		t.Fatalf("new daemon stop calls after stale event = %d, want 1", stopCalls)
	}
}

func TestObserveAuthProviderStopsDaemonWhenConfigurationIsMissing(t *testing.T) {
	d := &Dispatcher{ports: newPorts()}
	t.Cleanup(d.ports.daemonClose)
	authProvider := v1.AuthProvider{
		Name:       "entra",
		Namespace:  "default",
		Generation: 2,
		Annotations: map[string]string{
			v1.AuthProviderSyncAnnotation: "current",
		},
	}
	authProvider.Status.MissingConfigurationParameters = []string{"CLIENT_SECRET"}
	revision, err := authProviderRevision(authProvider)
	if err != nil {
		t.Fatal(err)
	}
	key := providerKeyForAuthProvider(authProvider.Namespace, authProvider.Name)
	stopCalls := 0
	d.ports.daemonPorts[key] = 10443
	d.ports.daemonsRunning[key] = func() {
		stopCalls++
	}
	d.ports.daemonRevisions[key] = revision

	if err := d.ObserveAuthProvider(authProvider); err != nil {
		t.Fatal(err)
	}
	if stopCalls != 1 {
		t.Fatalf("daemon stop calls = %d, want 1", stopCalls)
	}
	if _, running := d.ports.daemonsRunning[key]; running {
		t.Fatal("unconfigured daemon remains in registry")
	}
}

func TestObserveAuthProviderDoesNotStopNewerDaemonForStaleMissingStatus(t *testing.T) {
	d := &Dispatcher{ports: newPorts()}
	t.Cleanup(d.ports.daemonClose)
	key := providerKeyForAuthProvider("default", "entra")
	newRevision := daemonRevision{
		generation: 2,
		value:      "new",
	}
	stopCalls := 0
	d.ports.desiredDaemonRevisions[key] = newRevision
	d.ports.daemonPorts[key] = 10443
	d.ports.daemonsRunning[key] = func() {
		stopCalls++
	}
	d.ports.daemonRevisions[key] = newRevision
	staleAuthProvider := v1.AuthProvider{
		Name:       "entra",
		Namespace:  "default",
		Generation: 1,
		Annotations: map[string]string{
			v1.AuthProviderSyncAnnotation: "old",
		},
	}
	staleAuthProvider.Status.MissingConfigurationParameters = []string{"CLIENT_SECRET"}

	if err := d.ObserveAuthProvider(staleAuthProvider); err != nil {
		t.Fatal(err)
	}
	if stopCalls != 0 {
		t.Fatalf("daemon stop calls = %d, want 0", stopCalls)
	}
}

func TestStartDaemonRejectsSupersededRevision(t *testing.T) {
	d := &Dispatcher{ports: newPorts()}
	t.Cleanup(d.ports.daemonClose)
	key := providerKeyForAuthProvider("default", "entra")
	d.ports.desiredDaemonRevisions[key] = daemonRevision{
		generation: 2,
		value:      "new",
	}

	_, err := d.startDaemon(nil, key, daemonRevision{
		generation: 1,
		value:      "old",
	}, "command-that-must-not-run")
	if !errors.Is(err, errDaemonRevisionChanged) {
		t.Fatalf("startDaemon error = %v, want %v", err, errDaemonRevisionChanged)
	}
}

func TestForgetAuthProviderAllowsLowerGenerationAfterRecreation(t *testing.T) {
	d := &Dispatcher{ports: newPorts()}
	t.Cleanup(d.ports.daemonClose)
	key := providerKeyForAuthProvider("default", "entra")
	d.ports.desiredDaemonRevisions[key] = daemonRevision{
		instance:   "old-instance",
		generation: 10,
		value:      "old",
	}

	d.ForgetAuthProvider("default", "entra")
	if _, ok := d.ports.desiredDaemonRevisions[key]; ok {
		t.Fatal("deleted auth provider revision remains in registry")
	}

	if current := d.observeDaemonRevision(key, daemonRevision{
		instance:   "new-instance",
		generation: 1,
		value:      "new",
	}); !current {
		t.Fatal("recreated auth provider revision was rejected")
	}
}
