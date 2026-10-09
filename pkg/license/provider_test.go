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
	"sync/atomic"
	"testing"
	"time"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
)

// keygenResponder answers a fake Keygen request in place of the default response, and reports whether it did.
type keygenResponder func(http.ResponseWriter, *http.Request) bool

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
				_, _ = fmt.Fprint(w, validationResponseWithCode("license-1", "NO_MACHINE", false))
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

func TestUpdateRefreshesEntitlements(t *testing.T) {
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
	if err := provider.refresh(ctx, true); err != nil {
		t.Fatalf("expected provider update to succeed: %v", err)
	}

	if provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected old entitlement to be removed")
	}
	if !provider.hasEntitlement("OTHER_ENTITLEMENT") {
		t.Fatal("expected new entitlement to be added")
	}
}

func TestUpdateClearsEntitlementsWhenLicenseInvalid(t *testing.T) {
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
	if err := provider.refresh(ctx, true); err != nil {
		t.Fatalf("expected provider update to succeed: %v", err)
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

func TestKeygenOutageUsesStoredState(t *testing.T) {
	// Keep a hanging Keygen from slowing the test down.
	defer func(timeout time.Duration) { keygenTimeout = timeout }(keygenTimeout)
	keygenTimeout = time.Second

	for _, tt := range []struct {
		name    string
		respond keygenResponder
	}{
		{
			name:    "license lookup unavailable",
			respond: respondWithStatus("/v1/me", http.StatusServiceUnavailable),
		},
		{
			name:    "validation unavailable",
			respond: respondWithStatus("/v1/licenses/license-1/actions/validate", http.StatusServiceUnavailable),
		},
		{
			name:    "entitlements unavailable",
			respond: respondWithStatus("/v1/licenses/license-1/entitlements", http.StatusServiceUnavailable),
		},
		{
			name: "activation unavailable",
			respond: func(w http.ResponseWriter, r *http.Request) bool {
				switch r.URL.Path {
				case "/v1/licenses/license-1/actions/validate":
					_, _ = fmt.Fprint(w, validationResponseWithCode("license-1", "NO_MACHINE", false))
				case "/v1/machines":
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
				default:
					return false
				}
				return true
			},
		},
		{
			name: "keygen hangs",
			respond: func(_ http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/v1/me" {
					return false
				}
				<-r.Context().Done()
				return true
			},
		},
		{
			name:    "rate limited",
			respond: respondWithStatus("/v1/me", http.StatusTooManyRequests),
		},
		{
			name: "unexpected error document",
			respond: func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/v1/licenses/license-1/actions/validate" {
					return false
				}
				writeKeygenError(w, http.StatusBadRequest, "PARAMETER_INVALID")
				return true
			},
		},
		{
			name: "incomplete validation request",
			respond: func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/v1/licenses/license-1/actions/validate" {
					return false
				}
				_, _ = fmt.Fprint(w, validationResponseWithCode("license-1", "PRODUCT_SCOPE_REQUIRED", false))
				return true
			},
		},
		{
			name:    "empty validation response",
			respond: respondWithStatus("/v1/licenses/license-1/actions/validate", http.StatusNoContent),
		},
		{
			name: "connection dropped",
			respond: func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/v1/me" {
					return false
				}
				conn, _, err := http.NewResponseController(w).Hijack()
				if err != nil {
					t.Errorf("failed to hijack connection: %v", err)
					return true
				}
				_ = conn.Close()
				return true
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var failing atomic.Bool
			server := newTestKeygenServer(t, func(w http.ResponseWriter, r *http.Request) bool {
				return failing.Load() && tt.respond(w, r)
			})
			gatewayClient := newTestLicenseGatewayClient(t)
			provider := newTestProvider(t, gatewayClient, "license-key", server.URL)

			failing.Store(true)
			if err := provider.refresh(t.Context(), true); err != nil {
				t.Fatalf("expected the failed check to fall back: %v", err)
			}
			if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
				t.Fatal("expected the stored entitlements while Keygen can't answer")
			}

			// A pod that starts during the outage uses them too.
			restarted := newTestProvider(t, gatewayClient, "license-key", server.URL)
			if !restarted.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
				t.Fatal("expected a restarted pod to use the stored entitlements")
			}
		})
	}
}

func TestCanceledCheckKeepsCachedEntitlements(t *testing.T) {
	var hanging atomic.Bool
	server := newTestKeygenServer(t, func(_ http.ResponseWriter, r *http.Request) bool {
		if !hanging.Load() || r.URL.Path != "/v1/me" {
			return false
		}
		<-r.Context().Done()
		return true
	})
	provider := newTestProvider(t, newTestLicenseGatewayClient(t), "license-key", server.URL)

	hanging.Store(true)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := provider.refresh(ctx, true); err == nil {
		t.Fatal("expected the canceled check to be reported")
	}
	if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected the cached entitlements to stay when the caller gives up")
	}
}

func TestKeygenRejectionIsStored(t *testing.T) {
	for _, tt := range []struct {
		name    string
		respond keygenResponder
	}{
		{
			name: "license suspended",
			respond: func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/v1/licenses/license-1/actions/validate" {
					return false
				}
				writeKeygenError(w, http.StatusForbidden, "LICENSE_SUSPENDED")
				return true
			},
		},
		{
			name: "license key invalid",
			respond: func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/v1/me" {
					return false
				}
				writeKeygenError(w, http.StatusUnauthorized, "LICENSE_INVALID")
				return true
			},
		},
		{
			name: "license expired",
			respond: func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path != "/v1/licenses/license-1/actions/validate" {
					return false
				}
				_, _ = fmt.Fprint(w, validationResponseWithCode("license-1", "EXPIRED", false))
				return true
			},
		},
		{
			name: "machine limit exceeded",
			respond: func(w http.ResponseWriter, r *http.Request) bool {
				switch r.URL.Path {
				case "/v1/licenses/license-1/actions/validate":
					_, _ = fmt.Fprint(w, validationResponseWithCode("license-1", "NO_MACHINE", false))
				case "/v1/machines":
					writeKeygenError(w, http.StatusUnprocessableEntity, "MACHINE_LIMIT_EXCEEDED")
				default:
					return false
				}
				return true
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var rejecting, failing atomic.Bool
			server := newTestKeygenServer(t, func(w http.ResponseWriter, r *http.Request) bool {
				if failing.Load() {
					return respondWithStatus("/v1/me", http.StatusServiceUnavailable)(w, r)
				}
				return rejecting.Load() && tt.respond(w, r)
			})
			ctx := t.Context()
			gatewayClient := newTestLicenseGatewayClient(t)
			provider := newTestProvider(t, gatewayClient, "license-key", server.URL)

			rejecting.Store(true)
			if err := provider.refresh(ctx, true); err != nil {
				t.Fatalf("expected the rejection to be accepted: %v", err)
			}
			if requireValidLicense(ctx, t, provider) {
				t.Fatal("expected the rejected license to be invalid")
			}

			// The rejection replaced the stored answer, so a pod that starts during an outage doesn't fall back to an
			// older one.
			failing.Store(true)
			restarted := newTestProvider(t, gatewayClient, "license-key", server.URL)
			if requireValidLicense(ctx, t, restarted) {
				t.Fatal("expected a restarted pod to use the stored rejection")
			}
		})
	}
}

func TestUnreadableStoredStateKeepsCachedEntitlements(t *testing.T) {
	var failing atomic.Bool
	server := newTestKeygenServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		return failing.Load() && respondWithStatus("/v1/me", http.StatusServiceUnavailable)(w, r)
	})
	gatewayClient := newTestLicenseGatewayClient(t)
	provider := newTestProvider(t, gatewayClient, "license-key", server.URL)
	if _, err := gatewayClient.SetProperty(t.Context(), licenseStatePropertyKey("license-key"), "unreadable"); err != nil {
		t.Fatalf("failed to overwrite the stored license state: %v", err)
	}

	failing.Store(true)
	if err := provider.refresh(t.Context(), true); err != nil {
		t.Fatalf("expected the failed check to fall back: %v", err)
	}
	if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected the cached entitlements to stay when the stored state can't be read")
	}
}

func TestStoredStateKeepsLicenseWithoutEntitlements(t *testing.T) {
	var failing atomic.Bool
	server := newTestKeygenServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if failing.Load() {
			return respondWithStatus("/v1/me", http.StatusServiceUnavailable)(w, r)
		}
		if r.URL.Path != "/v1/licenses/license-1/entitlements" {
			return false
		}
		_, _ = fmt.Fprint(w, entitlementsResponse())
		return true
	})
	gatewayClient := newTestLicenseGatewayClient(t)
	newTestProvider(t, gatewayClient, "license-key", server.URL)

	failing.Store(true)
	restarted := newTestProvider(t, gatewayClient, "license-key", server.URL)
	if !requireValidLicense(t.Context(), t, restarted) {
		t.Fatal("expected a valid license without entitlements to stay valid")
	}
}

func TestStoredStateIsOnlyUsedForItsKey(t *testing.T) {
	var failing atomic.Bool
	server := newTestKeygenServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		return failing.Load() && respondWithStatus("/v1/me", http.StatusServiceUnavailable)(w, r)
	})
	gatewayClient := newTestLicenseGatewayClient(t)
	newTestProvider(t, gatewayClient, "license-key", server.URL)

	failing.Store(true)
	other := newTestProvider(t, gatewayClient, "other-license-key", server.URL)
	if requireValidLicense(t.Context(), t, other) {
		t.Fatal("expected another key not to use the stored answer")
	}
}

func TestKeyChangeDuringKeygenOutageUsesStoredState(t *testing.T) {
	var failing atomic.Bool
	server := newTestKeygenServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		return failing.Load() && respondWithStatus("/v1/me", http.StatusServiceUnavailable)(w, r)
	})
	ctx := t.Context()
	gatewayClient := newTestLicenseGatewayClient(t)
	replicaOne := newTestProvider(t, gatewayClient, "", server.URL)
	replicaTwo := newTestProvider(t, gatewayClient, "", server.URL)

	if err := replicaOne.SetLicenseKey(ctx, "license-key"); err != nil {
		t.Fatalf("expected license key to be stored: %v", err)
	}

	// The other replica checks the new key on its next read, and falls back to the answer stored with it.
	failing.Store(true)
	if !requireValidLicense(ctx, t, replicaTwo) {
		t.Fatal("expected the other replica to use the stored answer for the new key")
	}
}

func TestSetLicenseKeyReportsKeygenOutage(t *testing.T) {
	var rejecting atomic.Bool
	server := newTestKeygenServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/v1/me" {
			return false
		}
		if rejecting.Load() {
			writeKeygenError(w, http.StatusForbidden, "LICENSE_INVALID")
		} else {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}
		return true
	})
	ctx := t.Context()
	provider := newTestProvider(t, newTestLicenseGatewayClient(t), "", server.URL)

	err := provider.SetLicenseKey(ctx, "license-key")
	if err == nil || errors.Is(err, ErrInvalidLicense) {
		t.Fatalf("expected the outage to be reported, got %v", err)
	}

	rejecting.Store(true)
	if err := provider.SetLicenseKey(ctx, "license-key"); !errors.Is(err, ErrInvalidLicense) {
		t.Fatalf("expected %v, got %v", ErrInvalidLicense, err)
	}

	licenseKey, err := provider.LicenseKey(ctx)
	if err != nil {
		t.Fatalf("expected stored license key lookup to succeed: %v", err)
	}
	if licenseKey != "" {
		t.Fatalf("expected no license key to be stored, got %q", licenseKey)
	}
}

func respondWithStatus(path string, status int) keygenResponder {
	return func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != path {
			return false
		}
		http.Error(w, http.StatusText(status), status)
		return true
	}
}

func writeKeygenError(w http.ResponseWriter, status int, code string) {
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"errors":[{"title":"Rejected","detail":"license is rejected","code":%q}]}`, code)
}

// newTestKeygenServer returns a fake Keygen server that accepts license-1, unless override answers the request.
func newTestKeygenServer(t *testing.T, override keygenResponder) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if override(w, r) {
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

// newTestProvider starts a provider with the given configured license key, which checks it at startup.
func newTestProvider(t *testing.T, gatewayClient *gatewayclient.Client, licenseKey, apiURL string) *Provider {
	t.Helper()
	provider, err := newProvider(t.Context(), gatewayClient, Config{
		LicenseKey: licenseKey,
	}, apiURL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}
	return provider
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
	return validationResponseWithCode("license-1", "VALID", true)
}

func validationResponseWithCode(id, code string, valid bool) string {
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
}`, id, valid, code)
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
