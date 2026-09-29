package license

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
		t.Fatalf("expected one machine lookup during validation, got %d", lookups)
	}
	if lookedUpAs != provider.machineFingerprint {
		t.Fatalf("expected machine lookup by fingerprint %q, got %q", provider.machineFingerprint, lookedUpAs)
	}
}

func TestMachineIDLookupFailureKeepsLicenseValidAndRetries(t *testing.T) {
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

	if !requireValidLicense(ctx, t, provider) {
		t.Fatal("expected a failed machine lookup to leave the license valid")
	}
	if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected a failed machine lookup to leave entitlements intact")
	}

	if _, err := provider.MachineID(ctx); err == nil {
		t.Fatal("expected machine ID lookup to fail while Keygen is unavailable")
	}

	// A machine for another fingerprint does not identify this installation.
	status = http.StatusOK
	machineFingerprint = "another-installation"
	if _, err := provider.MachineID(ctx); err == nil {
		t.Fatal("expected machine ID lookup to reject another installation's machine")
	}

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
	if lookups != 4 {
		t.Fatalf("expected validation, two failed retries, and one successful retry, got %d lookups", lookups)
	}
	if !requireValidLicense(ctx, t, provider) {
		t.Fatal("expected license to remain valid")
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
