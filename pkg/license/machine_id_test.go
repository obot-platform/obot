package license

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testActivatedMachineID = "2c6ae1d4-5f0b-4a57-9d3e-7b8c1f2a4e60"
	testMachineID          = "9b1e7c3a-4d2f-4e8a-b6c5-1f0d3a2e7b94"
)

// newMachineLookupServer fakes Keygen for an already-activated license. lookup
// answers GET machines/<fingerprint> requests.
func newMachineLookupServer(t *testing.T, lookup func(w http.ResponseWriter, r *http.Request, fingerprint string)) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		if fingerprint, ok := strings.CutPrefix(r.URL.Path, "/v1/machines/"); ok && r.Method == http.MethodGet {
			lookup(w, r, fingerprint)
			return
		}

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(EnterpriseAuthProvidersEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

// expireMachineLookupBackoff lets the next MachineID call retry a failed lookup.
func expireMachineLookupBackoff(provider *Provider) {
	provider.lock.Lock()
	defer provider.lock.Unlock()
	provider.machineLookupAfter = time.Time{}
}

func TestMachineIDLooksUpActivatedMachineByFingerprint(t *testing.T) {
	var (
		lookups    int
		lookedUpAs string
	)
	server := newMachineLookupServer(t, func(w http.ResponseWriter, r *http.Request, fingerprint string) {
		lookups++
		lookedUpAs = fingerprint
		if r.Header.Get("Authorization") != "License license-key" {
			t.Errorf("machine lookup was not authenticated with the license key: %q", r.Header.Get("Authorization"))
		}
		_, _ = fmt.Fprint(w, machineResponse(strings.ToUpper(testMachineID), fingerprint))
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	provider, err := newProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	// Installations that never use the model proxy never look their machine up.
	if !requireValidLicense(ctx, t, provider) || lookups != 0 {
		t.Fatalf("expected a valid license without a machine lookup, got %d lookups", lookups)
	}

	for range 2 {
		machineID, err := provider.MachineID(ctx)
		if err != nil {
			t.Fatalf("expected machine ID lookup to succeed: %v", err)
		}
		// Keygen IDs are normalized to the canonical lowercase form the model proxy requires.
		if machineID != testMachineID {
			t.Fatalf("expected machine ID %q, got %q", testMachineID, machineID)
		}
	}

	if lookups != 1 {
		t.Fatalf("expected the first call to look the machine up and the second to use the cache, got %d lookups", lookups)
	}
	if lookedUpAs != provider.machineFingerprint {
		t.Fatalf("expected machine lookup by fingerprint %q, got %q", provider.machineFingerprint, lookedUpAs)
	}
}

func TestMachineIDLookupFailureKeepsLicenseValidAndBacksOff(t *testing.T) {
	var (
		lookups            int
		status             = http.StatusServiceUnavailable
		machineFingerprint string
	)
	server := newMachineLookupServer(t, func(w http.ResponseWriter, _ *http.Request, fingerprint string) {
		lookups++
		if status != http.StatusOK {
			http.Error(w, "temporarily unavailable", status)
			return
		}
		if machineFingerprint == "" {
			machineFingerprint = fingerprint
		}
		_, _ = fmt.Fprint(w, machineResponse(testMachineID, machineFingerprint))
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	provider, err := newProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	if _, err := provider.MachineID(ctx); err == nil {
		t.Fatal("expected machine ID lookup to fail while Keygen is unavailable")
	}
	if !requireValidLicense(ctx, t, provider) {
		t.Fatal("expected a failed machine lookup to leave the license valid")
	}
	if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected a failed machine lookup to leave entitlements intact")
	}

	// Requests during the backoff do not call Keygen again.
	if _, err := provider.MachineID(ctx); !errors.Is(err, errMachineLookupBackoff) {
		t.Fatalf("expected machine ID lookup to back off, got %v", err)
	}
	if lookups != 1 {
		t.Fatalf("expected one lookup before the backoff expires, got %d", lookups)
	}

	// A machine for another fingerprint does not identify this installation.
	expireMachineLookupBackoff(provider)
	status = http.StatusOK
	machineFingerprint = "another-installation"
	if _, err := provider.MachineID(ctx); err == nil || errors.Is(err, errMachineLookupBackoff) {
		t.Fatalf("expected machine ID lookup to reject another installation's machine, got %v", err)
	}

	expireMachineLookupBackoff(provider)
	machineFingerprint = ""
	machineID, err := provider.MachineID(ctx)
	if err != nil {
		t.Fatalf("expected machine ID lookup retry to succeed: %v", err)
	}
	if machineID != testMachineID {
		t.Fatalf("expected machine ID %q, got %q", testMachineID, machineID)
	}

	if _, err := provider.MachineID(ctx); err != nil {
		t.Fatalf("expected cached machine ID: %v", err)
	}
	if lookups != 3 {
		t.Fatalf("expected a failed, a rejected, and a successful lookup, got %d lookups", lookups)
	}
}

func TestMachineIDLookupIsSharedAndDoesNotBlockLicenseChanges(t *testing.T) {
	var (
		lookups atomic.Int32
		started = make(chan struct{})
		release = make(chan struct{})
	)
	server := newMachineLookupServer(t, func(w http.ResponseWriter, _ *http.Request, fingerprint string) {
		if lookups.Add(1) == 1 {
			close(started)
		}
		<-release
		_, _ = fmt.Fprint(w, machineResponse(testMachineID, fingerprint))
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	provider, err := newProvider(ctx, newTestLicenseGatewayClient(t), Config{}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}
	if err := provider.SetLicenseKey(ctx, "license-key"); err != nil {
		t.Fatalf("expected license key to be stored: %v", err)
	}

	const callers = 5
	type lookupResult struct {
		machineID string
		err       error
	}
	results := make(chan lookupResult, callers)
	for range callers {
		go func() {
			machineID, err := provider.MachineID(ctx)
			results <- lookupResult{machineID: machineID, err: err}
		}()
	}

	<-started
	// Let the other callers join the stalled lookup.
	time.Sleep(50 * time.Millisecond)

	// Removing the key needs refreshLock, which the stalled lookup must not hold.
	removed := make(chan error, 1)
	go func() { removed <- provider.RemoveLicenseKey(ctx) }()
	select {
	case err := <-removed:
		if err != nil {
			t.Fatalf("expected license key to be removed: %v", err)
		}
	case <-time.After(5 * time.Second):
		close(release)
		t.Fatal("machine lookup blocked license key removal")
	}
	close(release)

	// The lookup finished for a removed key, so no caller may receive its machine.
	for range callers {
		if result := <-results; result.machineID != "" {
			t.Fatalf("expected no machine ID for a removed license key, got %q", result.machineID)
		}
	}
	if lookups.Load() != 1 {
		t.Fatalf("expected concurrent callers to share one lookup, got %d", lookups.Load())
	}

	if machineID, err := provider.MachineID(ctx); err != nil || machineID != "" {
		t.Fatalf("expected no machine ID after the key was removed, got %q, %v", machineID, err)
	}
}

func TestMachineIDFollowsLicenseKeyChanges(t *testing.T) {
	machines := map[string]string{
		"License license-one": "0f5c2b8e-7a41-4d93-8e6b-5c2d1a9f3e07",
		"License license-two": "6e4a9d1c-2b7f-4c58-a3e0-8d1f6b2c9a45",
	}
	var lookups int
	server := newMachineLookupServer(t, func(w http.ResponseWriter, r *http.Request, fingerprint string) {
		lookups++
		machineID, ok := machines[r.Header.Get("Authorization")]
		if !ok {
			http.Error(w, "unexpected license key", http.StatusUnauthorized)
			return
		}
		_, _ = fmt.Fprint(w, machineResponse(machineID, fingerprint))
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	gatewayClient := newTestLicenseGatewayClient(t)
	replicaOne, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("expected first provider to be created: %v", err)
	}
	replicaTwo, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("expected second provider to be created: %v", err)
	}

	requireMachineID := func(provider *Provider, expected string) {
		t.Helper()

		machineID, err := provider.MachineID(ctx)
		if err != nil {
			t.Fatalf("expected machine ID lookup to succeed: %v", err)
		}
		if machineID != expected {
			t.Fatalf("expected machine ID %q, got %q", expected, machineID)
		}
	}

	requireMachineID(replicaOne, "")
	if lookups != 0 {
		t.Fatalf("expected no machine lookup without a license, got %d", lookups)
	}

	for _, key := range []string{"license-one", "license-two"} {
		if err := replicaOne.SetLicenseKey(ctx, key); err != nil {
			t.Fatalf("expected license key %q to be stored: %v", key, err)
		}
		requireMachineID(replicaOne, machines["License "+key])
		requireMachineID(replicaTwo, machines["License "+key])
	}

	if err := replicaOne.RemoveLicenseKey(ctx); err != nil {
		t.Fatalf("expected license key to be removed: %v", err)
	}
	requireMachineID(replicaOne, "")
	requireMachineID(replicaTwo, "")
}
