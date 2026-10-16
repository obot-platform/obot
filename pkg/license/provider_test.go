package license

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
)

// fakeKeygen answers license checks for license-1 with a valid license granting EnterpriseAuthProvidersEntitlement,
// unless it's set to fail or reject them.
type fakeKeygen struct {
	// fail, if set, handles every request instead.
	fail http.HandlerFunc

	// meStatus and meCode, if meCode is set, are the error /v1/me answers with.
	meStatus int
	meCode   string

	// validationCode, if set, is the code of a valid:false validation result.
	validationCode string

	// activationCode, if set, is the error code machine activation is refused with.
	activationCode string

	// expiry, if set, is the license's expiry.
	expiry *time.Time
}

func (k *fakeKeygen) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/vnd.api+json")
	if k.fail != nil {
		k.fail(w, r)
		return
	}

	switch r.URL.Path {
	case "/v1/me":
		if k.meCode != "" {
			w.WriteHeader(k.meStatus)
			_, _ = fmt.Fprintf(w, `{"errors":[{"title":"Rejected","code":%q}]}`, k.meCode)
			return
		}
		_, _ = fmt.Fprint(w, licenseResponse())
	case "/v1/licenses/license-1/actions/validate":
		if k.validationCode != "" {
			_, _ = fmt.Fprint(w, validationResponseWithCode("license-1", k.validationCode, false, nil))
			return
		}
		_, _ = fmt.Fprint(w, validationResponseWithCode("license-1", "VALID", true, k.expiry))
	case "/v1/machines":
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = fmt.Fprintf(w, `{"errors":[{"title":"Unprocessable resource","code":%q}]}`, k.activationCode)
	case "/v1/licenses/license-1/entitlements":
		_, _ = fmt.Fprint(w, entitlementsResponse(EnterpriseAuthProvidersEntitlement))
	default:
		http.NotFound(w, r)
	}
}

func unavailable(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
}

func newTestProvider(ctx context.Context, t *testing.T, gatewayClient *gatewayclient.Client, url string) *Provider {
	t.Helper()

	provider, err := newProvider(ctx, gatewayClient, Config{
		LicenseKey: "license-key",
	}, url)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}
	return provider
}

func requireValidLicense(ctx context.Context, t *testing.T, provider *Provider) bool {
	t.Helper()
	valid, err := provider.HasValidLicense(ctx)
	if err != nil {
		t.Fatalf("expected license state lookup to succeed: %v", err)
	}
	return valid
}

func TestRequireEntitlement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

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
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	provider, err := newProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	if !requireValidLicense(ctx, t, provider) {
		t.Fatal("expected license to be valid")
	}
	if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected entitlement to be accepted")
	}
	entitlements, err := provider.Entitlements(ctx)
	if err != nil {
		t.Fatalf("expected entitlements lookup to succeed: %v", err)
	}
	if len(entitlements) != 1 || entitlements[0] != EnterpriseAuthProvidersEntitlement {
		t.Fatalf("expected entitlement list to contain %q, got %v", EnterpriseAuthProvidersEntitlement, entitlements)
	}
}

func TestRequireEntitlementMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse("OTHER_ENTITLEMENT"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	provider, err := newProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	if !requireValidLicense(ctx, t, provider) {
		t.Fatal("expected license to be valid")
	}
	if provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected entitlement to be missing")
	}
}

func TestNewProviderNotConfigured(t *testing.T) {
	provider, err := NewProvider(t.Context(), nil, Config{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider to be created")
	}
	if requireValidLicense(t.Context(), t, provider) {
		t.Fatal("expected license to be invalid")
	}
}

func TestNewProviderContinuesWhenInitialRefreshFails(t *testing.T) {
	refreshFails := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			if refreshFails {
				http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(EnterpriseAuthProvidersEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	provider, err := newProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created despite refresh failure: %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider to be created")
	}
	if requireValidLicense(ctx, t, provider) {
		t.Fatal("expected provider to start unlicensed")
	}
	if provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected provider to start without entitlements")
	}

	refreshFails = false
	if err := provider.Validate(ctx); err != nil {
		t.Fatalf("expected subsequent refresh to succeed: %v", err)
	}
	if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected entitlement after successful refresh")
	}
}

func TestNewProviderActivatesLicenseOnNoMachine(t *testing.T) {
	machineFingerprint := ""
	validationCount := 0
	activated := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			validationCount++
			machineFingerprint = assertValidateFingerprint(t, r, machineFingerprint)
			if validationCount == 1 {
				_, _ = fmt.Fprint(w, validationResponseWithCode("license-1", "NO_MACHINE", false, nil))
				return
			}
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/machines":
			activated = true
			assertActivationFingerprint(t, r, machineFingerprint)
			_, _ = fmt.Fprint(w, machineResponse("machine-1", machineFingerprint))
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(EnterpriseAuthProvidersEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	provider, err := newProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	if !activated {
		t.Fatal("expected license to be activated")
	}
	if validationCount != 2 {
		t.Fatalf("expected license to be validated before and after activation, got %d validations", validationCount)
	}
	if !requireValidLicense(ctx, t, provider) {
		t.Fatal("expected license to be valid after activation")
	}
	if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected entitlement to be accepted")
	}
}

func TestValidateRefreshesEntitlements(t *testing.T) {
	entitlement := EnterpriseAuthProvidersEntitlement
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(entitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	provider, err := newProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	entitlement = "OTHER_ENTITLEMENT"
	if err := provider.Validate(ctx); err != nil {
		t.Fatalf("expected validation to succeed: %v", err)
	}

	if provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected old entitlement to be removed")
	}
	if !provider.hasEntitlement("OTHER_ENTITLEMENT") {
		t.Fatal("expected new entitlement to be added")
	}
}

func TestValidateClearsEntitlementsWhenLicenseInvalid(t *testing.T) {
	invalid := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			if invalid {
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"errors":[{"title":"Forbidden","detail":"license is invalid","code":"LICENSE_INVALID"}]}`)
				return
			}
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(EnterpriseAuthProvidersEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	provider, err := newProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	invalid = true
	if err := provider.Validate(ctx); err != nil {
		t.Fatalf("expected validation to succeed: %v", err)
	}

	if requireValidLicense(ctx, t, provider) {
		t.Fatal("expected license to be marked invalid")
	}
	if provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected entitlement to be cleared")
	}
	entitlements, err := provider.Entitlements(ctx)
	if err != nil {
		t.Fatalf("expected entitlements lookup to succeed: %v", err)
	}
	if len(entitlements) != 0 {
		t.Fatalf("expected entitlements to be cleared, got %v", entitlements)
	}
}

func TestProviderRefreshesDatabaseLicenseAcrossReplicas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			switch r.Header.Get("Authorization") {
			case "License license-one":
				_, _ = fmt.Fprint(w, entitlementsResponse("ENTITLEMENT_ONE"))
			case "License license-two":
				_, _ = fmt.Fprint(w, entitlementsResponse("ENTITLEMENT_TWO"))
			default:
				http.Error(w, "unexpected license key", http.StatusUnauthorized)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

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

	if err := replicaOne.SetLicenseKey(ctx, "license-one"); err != nil {
		t.Fatalf("expected first license key to be stored: %v", err)
	}
	entitlements, err := replicaTwo.Entitlements(ctx)
	if err != nil {
		t.Fatalf("expected second replica to load first license: %v", err)
	}
	if len(entitlements) != 1 || entitlements[0] != "ENTITLEMENT_ONE" {
		t.Fatalf("expected second replica to use first license, got %v", entitlements)
	}

	if err := replicaOne.SetLicenseKey(ctx, "license-two"); err != nil {
		t.Fatalf("expected replacement license key to be stored: %v", err)
	}
	entitlements, err = replicaTwo.Entitlements(ctx)
	if err != nil {
		t.Fatalf("expected second replica to refresh replacement license: %v", err)
	}
	if len(entitlements) != 1 || entitlements[0] != "ENTITLEMENT_TWO" {
		t.Fatalf("expected second replica to use replacement license, got %v", entitlements)
	}

	if err := replicaOne.RemoveLicenseKey(ctx); err != nil {
		t.Fatalf("expected license key to be removed: %v", err)
	}
	if requireValidLicense(ctx, t, replicaTwo) {
		t.Fatal("expected second replica to clear its cached license after removal")
	}
	entitlements, err = replicaTwo.Entitlements(ctx)
	if err != nil {
		t.Fatalf("expected second replica entitlement lookup after removal to succeed: %v", err)
	}
	if len(entitlements) != 0 {
		t.Fatalf("expected second replica entitlements to be cleared, got %v", entitlements)
	}
}

func TestCachedSnapshotMatchesEquivalentTimestamps(t *testing.T) {
	updatedAt := time.Now()
	provider := &Provider{
		licenseKeySnapshot: licenseKeySnapshot{
			key:       "license-key",
			updatedAt: updatedAt,
		},
	}

	// Database round trips strip time.Time's monotonic clock reading.
	databaseSnapshot := licenseKeySnapshot{
		key:       "license-key",
		updatedAt: updatedAt.Round(0),
	}
	if !provider.cachedSnapshotMatches(databaseSnapshot) {
		t.Fatal("expected snapshots representing the same timestamp to match")
	}
}

func TestSetLicenseKeyWaitsForRefreshBeforeCommitting(t *testing.T) {
	validationCompleted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse("NEW_ENTITLEMENT"))
			close(validationCompleted)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	provider, err := newProvider(ctx, newTestLicenseGatewayClient(t), Config{}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	// Model a refresh that is still validating the previous database snapshot.
	provider.refreshLock.Lock()
	setDone := make(chan error, 1)
	go func() {
		setDone <- provider.SetLicenseKey(ctx, "new-license")
	}()

	select {
	case <-validationCompleted:
	case <-time.After(5 * time.Second):
		provider.refreshLock.Unlock()
		t.Fatal("timed out waiting for candidate license validation")
	}

	select {
	case err := <-setDone:
		provider.refreshLock.Unlock()
		if err != nil {
			t.Fatalf("expected license key update to succeed: %v", err)
		}
		t.Fatal("SetLicenseKey committed while a refresh was in progress")
	case <-time.After(500 * time.Millisecond):
		provider.refreshLock.Unlock()
	}

	if err := <-setDone; err != nil {
		t.Fatalf("expected license key update to succeed after refresh: %v", err)
	}
	licenseKey, err := provider.LicenseKey(ctx)
	if err != nil {
		t.Fatalf("expected stored license key lookup to succeed: %v", err)
	}
	if licenseKey != "new-license" {
		t.Fatalf("expected new license key to be stored, got %q", licenseKey)
	}
	if !provider.hasEntitlement("NEW_ENTITLEMENT") {
		t.Fatal("expected new license entitlements to be cached")
	}
}

func TestRemoveLicenseKeyWaitsForRefreshBeforeClearingState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse("OLD_ENTITLEMENT"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	provider, err := newProvider(ctx, newTestLicenseGatewayClient(t), Config{}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}
	if err := provider.SetLicenseKey(ctx, "old-license"); err != nil {
		t.Fatalf("expected initial license key to be stored: %v", err)
	}

	// Model a refresh that is still validating the license being removed.
	provider.refreshLock.Lock()
	removeDone := make(chan error, 1)
	go func() {
		removeDone <- provider.RemoveLicenseKey(ctx)
	}()

	select {
	case err := <-removeDone:
		provider.refreshLock.Unlock()
		if err != nil {
			t.Fatalf("expected license key removal to succeed: %v", err)
		}
		t.Fatal("RemoveLicenseKey cleared state while a refresh was in progress")
	case <-time.After(500 * time.Millisecond):
		provider.refreshLock.Unlock()
	}

	if err := <-removeDone; err != nil {
		t.Fatalf("expected license key removal to succeed after refresh: %v", err)
	}
	licenseKey, err := provider.LicenseKey(ctx)
	if err != nil {
		t.Fatalf("expected stored license key lookup to succeed: %v", err)
	}
	if licenseKey != "" {
		t.Fatalf("expected license key to be removed, got %q", licenseKey)
	}
	if provider.hasEntitlement("OLD_ENTITLEMENT") {
		t.Fatal("expected removed license entitlements to be cleared")
	}
}

func TestKeygenFailuresKeepEntitlements(t *testing.T) {
	tests := []struct {
		name string
		fail http.HandlerFunc
	}{
		{
			name: "server error",
			fail: unavailable,
		},
		{
			name: "rate limited",
			fail: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = fmt.Fprint(w, `{"errors":[{"title":"Too many requests","code":"TOO_MANY_REQUESTS"}]}`)
			},
		},
		{
			name: "Keygen account subscription inactive",
			fail: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				_, _ = fmt.Fprint(w, `{"errors":[{"title":"Account does not have an active subscription"}]}`)
			},
		},
		{
			name: "Keygen account request limit reached",
			fail: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusPaymentRequired)
				_, _ = fmt.Fprint(w, `{"errors":[{"title":"Daily API request limit reached"}]}`)
			},
		},
		{
			name: "Keygen account not found",
			fail: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"errors":[{"title":"Not found","code":"NOT_FOUND"}]}`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keygen := &fakeKeygen{}
			server := httptest.NewServer(keygen)
			defer server.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			gatewayClient := newTestLicenseGatewayClient(t)

			provider := newTestProvider(ctx, t, gatewayClient, server.URL)

			keygen.fail = tt.fail
			if err := provider.Validate(ctx); !errors.Is(err, ErrLicenseCheckFailed) {
				t.Fatalf("expected the failed check to be reported, got %v", err)
			}
			if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
				t.Fatal("expected entitlement to be kept")
			}

			// A replica starting while Keygen fails should use them too.
			restarted := newTestProvider(ctx, t, gatewayClient, server.URL)
			if !restarted.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
				t.Fatal("expected restarted provider to use the saved entitlements")
			}
		})
	}
}

func TestKeygenFailuresDropExpiredLicense(t *testing.T) {
	tests := []struct {
		name   string
		expiry time.Time
		want   bool
	}{
		{
			name:   "expired",
			expiry: time.Now().Add(-time.Hour),
			want:   false,
		},
		{
			name:   "not expired",
			expiry: time.Now().Add(time.Hour),
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keygen := &fakeKeygen{expiry: &tt.expiry}
			server := httptest.NewServer(keygen)
			defer server.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			gatewayClient := newTestLicenseGatewayClient(t)

			provider := newTestProvider(ctx, t, gatewayClient, server.URL)
			if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
				t.Fatal("expected Keygen's verdict to be used while it's reachable")
			}

			keygen.fail = unavailable
			if err := provider.Validate(ctx); !errors.Is(err, ErrLicenseCheckFailed) {
				t.Fatalf("expected the failed check to be reported, got %v", err)
			}
			if got := provider.hasEntitlement(EnterpriseAuthProvidersEntitlement); got != tt.want {
				t.Fatalf("expected entitlement kept to be %t, got %t", tt.want, got)
			}

			restarted := newTestProvider(ctx, t, gatewayClient, server.URL)
			if got := restarted.hasEntitlement(EnterpriseAuthProvidersEntitlement); got != tt.want {
				t.Fatalf("expected restarted provider's entitlement kept to be %t, got %t", tt.want, got)
			}
		})
	}
}

func TestKeygenRejectionsDegrade(t *testing.T) {
	tests := []struct {
		name   string
		reject func(*fakeKeygen)
	}{
		{
			name: "invalid key",
			reject: func(k *fakeKeygen) {
				k.meStatus, k.meCode = http.StatusUnauthorized, "LICENSE_INVALID"
			},
		},
		{
			name: "banned",
			reject: func(k *fakeKeygen) {
				k.meStatus, k.meCode = http.StatusForbidden, "LICENSE_BANNED"
			},
		},
		{
			name: "suspended",
			reject: func(k *fakeKeygen) {
				k.meStatus, k.meCode = http.StatusForbidden, "LICENSE_SUSPENDED"
			},
		},
		{
			name: "expired",
			reject: func(k *fakeKeygen) {
				k.meStatus, k.meCode = http.StatusForbidden, "LICENSE_EXPIRED"
			},
		},
		{
			name: "license authentication not allowed",
			reject: func(k *fakeKeygen) {
				k.meStatus, k.meCode = http.StatusForbidden, "LICENSE_NOT_ALLOWED"
			},
		},
		{
			name: "validation failed",
			reject: func(k *fakeKeygen) {
				k.validationCode = "SUSPENDED"
			},
		},
		{
			name: "machine limit",
			reject: func(k *fakeKeygen) {
				k.validationCode, k.activationCode = "NO_MACHINE", "MACHINE_LIMIT_EXCEEDED"
			},
		},
		{
			name: "core limit",
			reject: func(k *fakeKeygen) {
				k.validationCode, k.activationCode = "NO_MACHINE", "MACHINE_CORE_LIMIT_EXCEEDED"
			},
		},
		{
			name: "group machine limit",
			reject: func(k *fakeKeygen) {
				k.validationCode, k.activationCode = "NO_MACHINE", "GROUP_MACHINE_LIMIT_EXCEEDED"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keygen := &fakeKeygen{}
			server := httptest.NewServer(keygen)
			defer server.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			gatewayClient := newTestLicenseGatewayClient(t)

			provider := newTestProvider(ctx, t, gatewayClient, server.URL)

			tt.reject(keygen)
			if err := provider.Validate(ctx); err != nil {
				t.Fatalf("expected the rejection to be a successful check: %v", err)
			}
			if requireValidLicense(ctx, t, provider) {
				t.Fatal("expected rejected license to be invalid")
			}

			// A replica starting while Keygen is down shouldn't bring back the rejected license.
			keygen.fail = unavailable
			restarted := newTestProvider(ctx, t, gatewayClient, server.URL)
			if requireValidLicense(ctx, t, restarted) {
				t.Fatal("expected rejected license to stay invalid after restart")
			}
		})
	}
}

func TestSetLicenseKeyRequiresKeygenVerdict(t *testing.T) {
	keygen := &fakeKeygen{}
	server := httptest.NewServer(keygen)
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	provider, err := newProvider(ctx, newTestLicenseGatewayClient(t), Config{}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	keygen.fail = unavailable
	err = provider.SetLicenseKey(ctx, "license-key")
	if !errors.Is(err, ErrLicenseCheckFailed) {
		t.Fatalf("expected setting a key while Keygen is unavailable to fail with the check error, got %v", err)
	}
	if licenseKey, err := provider.LicenseKey(ctx); err != nil || licenseKey != "" {
		t.Fatalf("expected license key not to be stored, got %q, %v", licenseKey, err)
	}
}

func TestValidateKeepsCachedStateWhenSavedEntitlementsUnreadable(t *testing.T) {
	keygen := &fakeKeygen{}
	server := httptest.NewServer(keygen)
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	gatewayClient := newTestLicenseGatewayClient(t)

	provider := newTestProvider(ctx, t, gatewayClient, server.URL)
	if _, err := gatewayClient.SetProperty(ctx, licenseStatePropertyKey("license-key"), "corrupt"); err != nil {
		t.Fatalf("failed to corrupt saved entitlements: %v", err)
	}

	keygen.fail = unavailable
	if err := provider.Validate(ctx); err == nil {
		t.Fatal("expected validation to report the unreadable saved entitlements")
	}
	if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected cached entitlements to be kept")
	}
}

func newTestLicenseGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()

	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("failed to create storage services: %v", err)
	}
	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway database: %v", err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate gateway database: %v", err)
	}

	gatewayClient := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() {
		if err := gatewayClient.Close(); err != nil {
			t.Errorf("failed to close gateway client: %v", err)
		}
	})
	return gatewayClient
}

func licenseResponse() string {
	return fmt.Sprintf(`{
  "data": {
    "id": %q,
    "type": "licenses",
    "attributes": {
      "name": "Test License",
      "key": "license-key",
      "expiry": null,
      "scheme": null,
      "requireHeartbeat": false,
      "lastValidated": null,
      "metadata": {},
      "created": "2026-01-01T00:00:00Z",
      "updated": "2026-01-01T00:00:00Z"
    },
    "relationships": {
      "policy": {"data": {"type": "policies", "id": "policy-1"}}
    }
  }
}`, "license-1")
}

func validationResponse() string {
	return validationResponseWithCode("license-1", "VALID", true, nil)
}

func validationResponseWithCode(id, code string, valid bool, expiry *time.Time) string {
	expiryJSON, _ := json.Marshal(expiry)
	return fmt.Sprintf(`{
  "data": {
    "id": %q,
    "type": "licenses",
    "attributes": {
      "name": "Test License",
      "key": "license-key",
      "expiry": %s,
      "scheme": null,
      "requireHeartbeat": false,
      "lastValidated": "2026-01-01T00:00:00Z",
      "metadata": {},
      "created": "2026-01-01T00:00:00Z",
      "updated": "2026-01-01T00:00:00Z"
    },
    "relationships": {
      "policy": {"data": {"type": "policies", "id": "policy-1"}}
    }
  },
  "meta": {
    "valid": %t,
    "code": %q,
    "detail": "validation result"
  }
}`, id, expiryJSON, valid, code)
}

func machineResponse(id, fingerprint string) string {
	return fmt.Sprintf(`{
  "data": {
    "id": %q,
    "type": "machines",
    "attributes": {
      "fingerprint": %q,
      "hostname": "test-host",
      "platform": "darwin/arm64",
      "ip": "127.0.0.1",
      "cores": 1,
      "requireHeartbeat": false,
      "heartbeatStatus": "NOT_STARTED",
      "heartbeatDuration": 0,
      "metadata": {},
      "created": "2026-01-01T00:00:00Z",
      "updated": "2026-01-01T00:00:00Z"
    }
  }
}`, id, fingerprint)
}

func assertValidateFingerprint(t *testing.T, r *http.Request, expected string) string {
	t.Helper()

	body := decodeRequestBody(t, r)
	scope, ok := body["meta"].(map[string]any)["scope"].(map[string]any)
	if !ok {
		t.Fatalf("expected validate request scope, got %#v", body)
	}
	fingerprint, _ := scope["fingerprint"].(string)
	if product, _ := scope["product"].(string); product != keygenProduct {
		t.Fatalf("expected validate product %q, got %q", keygenProduct, product)
	}
	if expected == "" {
		if fingerprint == "" {
			t.Fatal("expected validate fingerprint to be set")
		}
		return fingerprint
	}
	if fingerprint != expected {
		t.Fatalf("expected validate fingerprint %q, got %q", expected, fingerprint)
	}
	return fingerprint
}

func assertActivationFingerprint(t *testing.T, r *http.Request, expected string) {
	t.Helper()

	body := decodeRequestBody(t, r)
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected activation request data, got %#v", body)
	}
	attributes, ok := data["attributes"].(map[string]any)
	if !ok {
		t.Fatalf("expected activation request attributes, got %#v", body)
	}
	if fingerprint, _ := attributes["fingerprint"].(string); fingerprint != expected {
		t.Fatalf("expected activation fingerprint %q, got %q", expected, fingerprint)
	}
}

func decodeRequestBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("failed to read request body: %v", err)
	}

	var body map[string]any
	if err := json.Unmarshal(bodyBytes, &body); err != nil {
		t.Fatalf("failed to decode request body %q: %v", string(bodyBytes), err)
	}
	return body
}

func entitlementsResponse(codes ...string) string {
	var data strings.Builder
	for i, code := range codes {
		if i > 0 {
			data.WriteByte(',')
		}
		_, _ = fmt.Fprintf(&data, `{
  "id": "entitlement-%d",
  "type": "entitlements",
  "attributes": {
    "code": %q,
    "metadata": {},
    "created": "2026-01-01T00:00:00Z",
    "updated": "2026-01-01T00:00:00Z"
  }
}`, i, code)
	}
	return fmt.Sprintf(`{"data":[%s]}`, data.String())
}
