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
	"gorm.io/gorm"
)

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
	if err := provider.update(ctx); err != nil {
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
	if err := provider.update(ctx); err != nil {
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

func TestRemoveEnterpriseLicenseFallsBackToCommunityLicense(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			switch r.Header.Get("Authorization") {
			case "License community-license":
				_, _ = fmt.Fprint(w, entitlementsResponse(CommunityEntitlement))
			case "License enterprise-license", "License replacement-enterprise-license":
				_, _ = fmt.Fprint(w, entitlementsResponse(CommunityEntitlement, EnterpriseEntitlement))
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
	provider, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}
	secondReplica, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("expected second provider to be created: %v", err)
	}

	if err := provider.SetLicenseKey(ctx, "community-license"); err != nil {
		t.Fatalf("expected community license key to be stored: %v", err)
	}
	if err := secondReplica.SetLicenseKey(ctx, "enterprise-license"); err != nil {
		t.Fatalf("expected enterprise license key to be stored: %v", err)
	}
	if err := provider.SetLicenseKey(ctx, "replacement-enterprise-license"); err != nil {
		t.Fatalf("expected replacement enterprise license key to be stored: %v", err)
	}

	if err := secondReplica.RemoveLicenseKey(ctx); err != nil {
		t.Fatalf("expected enterprise license key to be removed: %v", err)
	}

	licenseKey, err := provider.LicenseKey(ctx)
	if err != nil {
		t.Fatalf("expected fallback license key lookup to succeed: %v", err)
	}
	if licenseKey != "community-license" {
		t.Fatalf("license key = %q, want community fallback", licenseKey)
	}
	entitlements, err := provider.Entitlements(ctx)
	if err != nil {
		t.Fatalf("expected fallback entitlements lookup to succeed: %v", err)
	}
	if len(entitlements) != 1 || entitlements[0] != CommunityEntitlement {
		t.Fatalf("entitlements = %v, want [%s]", entitlements, CommunityEntitlement)
	}

	if err := provider.RemoveLicenseKey(ctx); err != nil {
		t.Fatalf("expected community fallback to remain removable: %v", err)
	}
	licenseKey, err = provider.LicenseKey(ctx)
	if err != nil {
		t.Fatalf("expected empty license key lookup to succeed: %v", err)
	}
	if licenseKey != "" {
		t.Fatalf("license key = %q after deleting fallback, want empty", licenseKey)
	}
}

func TestSetCommunityLicenseKeyReplacesPrimary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(CommunityEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := t.Context()
	gatewayClient := newTestLicenseGatewayClient(t)
	if _, err := gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, "invalid-primary"); err != nil {
		t.Fatalf("seed primary license: %v", err)
	}
	provider, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	if err := provider.SetCommunityLicenseKey(ctx, "community-license"); err != nil {
		t.Fatalf("SetCommunityLicenseKey(): %v", err)
	}
	if _, err := gatewayClient.GetProperty(ctx, LicenseKeyPropertyKey); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("primary property error = %v, want record not found", err)
	}
	community, err := gatewayClient.GetProperty(ctx, CommunityLicenseKeyPropertyKey)
	if err != nil {
		t.Fatalf("get Community property: %v", err)
	}
	if community.Value != "community-license" {
		t.Fatalf("Community property = %q, want community-license", community.Value)
	}
}

func TestRemoveLicenseKeyKeepsPrimaryWhenFallbackValidationFails(t *testing.T) {
	failCommunityValidation := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			if r.Header.Get("Authorization") == "License community-license" && failCommunityValidation {
				http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			_, _ = fmt.Fprint(w, entitlementsResponse(CommunityEntitlement, EnterpriseEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := t.Context()
	gatewayClient := newTestLicenseGatewayClient(t)
	if _, err := gatewayClient.SetProperty(ctx, CommunityLicenseKeyPropertyKey, "community-license"); err != nil {
		t.Fatalf("seed Community license: %v", err)
	}
	if _, err := gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, "enterprise-license"); err != nil {
		t.Fatalf("seed primary license: %v", err)
	}
	provider, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	if err := provider.RemoveLicenseKey(ctx); err == nil {
		t.Fatal("RemoveLicenseKey() error = nil, want fallback validation failure")
	}
	licenseKey, err := provider.LicenseKey(ctx)
	if err != nil {
		t.Fatalf("get license key after failed removal: %v", err)
	}
	if licenseKey != "enterprise-license" {
		t.Fatalf("license key after failed removal = %q, want enterprise-license", licenseKey)
	}

	failCommunityValidation = false
	if err := provider.RemoveLicenseKey(ctx); err != nil {
		t.Fatalf("retry RemoveLicenseKey(): %v", err)
	}
	licenseKey, err = provider.LicenseKey(ctx)
	if err != nil {
		t.Fatalf("get fallback license key: %v", err)
	}
	if licenseKey != "community-license" {
		t.Fatalf("fallback license key = %q, want community-license", licenseKey)
	}
}

func TestRemoveLicenseKeyAllowsInvalidFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			if r.Header.Get("Authorization") == "License invalid-community-license" {
				_, _ = fmt.Fprint(w, validationResponseWithCode("license-1", "EXPIRED", false))
				return
			}
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(CommunityEntitlement, EnterpriseEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := t.Context()
	gatewayClient := newTestLicenseGatewayClient(t)
	if _, err := gatewayClient.SetProperty(ctx, CommunityLicenseKeyPropertyKey, "invalid-community-license"); err != nil {
		t.Fatalf("seed invalid Community fallback: %v", err)
	}
	if _, err := gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, "enterprise-license"); err != nil {
		t.Fatalf("seed primary license: %v", err)
	}
	provider, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	if err := provider.RemoveLicenseKey(ctx); err != nil {
		t.Fatalf("remove primary license: %v", err)
	}
	licenseKey, err := provider.LicenseKey(ctx)
	if err != nil {
		t.Fatalf("get invalid fallback: %v", err)
	}
	if licenseKey != "invalid-community-license" {
		t.Fatalf("fallback license key = %q, want invalid-community-license", licenseKey)
	}
	if requireValidLicense(ctx, t, provider) {
		t.Fatal("invalid fallback reported as valid")
	}

	if err := provider.RemoveLicenseKey(ctx); err != nil {
		t.Fatalf("remove invalid fallback: %v", err)
	}
	licenseKey, err = provider.LicenseKey(ctx)
	if err != nil {
		t.Fatalf("get license key after deleting fallback: %v", err)
	}
	if licenseKey != "" {
		t.Fatalf("license key after deleting fallback = %q, want empty", licenseKey)
	}
}

func TestSetLicenseKeyRefusesToOverwriteIndeterminateLegacyLicense(t *testing.T) {
	communityLookupFails := true
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			if r.Header.Get("Authorization") == "License community-license" && communityLookupFails {
				http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
				return
			}
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			if r.Header.Get("Authorization") == "License community-license" {
				_, _ = fmt.Fprint(w, entitlementsResponse(CommunityEntitlement))
				return
			}
			_, _ = fmt.Fprint(w, entitlementsResponse(CommunityEntitlement, EnterpriseEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := t.Context()
	gatewayClient := newTestLicenseGatewayClient(t)
	if _, err := gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, "community-license"); err != nil {
		t.Fatalf("seed legacy Community license: %v", err)
	}
	provider, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	if err := provider.SetLicenseKey(ctx, "enterprise-license"); err == nil {
		t.Fatal("SetLicenseKey() error = nil, want indeterminate current-license error")
	}
	property, err := gatewayClient.GetProperty(ctx, LicenseKeyPropertyKey)
	if err != nil {
		t.Fatalf("get primary property: %v", err)
	}
	if property.Value != "community-license" {
		t.Fatalf("primary property after failed update = %q, want community-license", property.Value)
	}

	communityLookupFails = false
	if err := provider.SetLicenseKey(ctx, "enterprise-license"); err != nil {
		t.Fatalf("retry SetLicenseKey(): %v", err)
	}
	if err := provider.RemoveLicenseKey(ctx); err != nil {
		t.Fatalf("RemoveLicenseKey(): %v", err)
	}
	licenseKey, err := provider.LicenseKey(ctx)
	if err != nil {
		t.Fatalf("get preserved legacy Community license: %v", err)
	}
	if licenseKey != "community-license" {
		t.Fatalf("preserved license key = %q, want community-license", licenseKey)
	}
}

func TestSetLicenseKeyTreatsRequestTimeoutAsIndeterminate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			if r.Header.Get("Authorization") == "License community-license" {
				w.WriteHeader(http.StatusRequestTimeout)
				_, _ = fmt.Fprint(w, `{"errors":[{"title":"Request Timeout","detail":"temporarily unavailable","code":"REQUEST_TIMEOUT"}]}`)
				return
			}
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(CommunityEntitlement, EnterpriseEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := t.Context()
	gatewayClient := newTestLicenseGatewayClient(t)
	if _, err := gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, "community-license"); err != nil {
		t.Fatalf("seed legacy Community license: %v", err)
	}
	provider, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	if err := provider.SetLicenseKey(ctx, "enterprise-license"); err == nil {
		t.Fatal("SetLicenseKey() error = nil, want request timeout")
	}
	property, err := gatewayClient.GetProperty(ctx, LicenseKeyPropertyKey)
	if err != nil {
		t.Fatalf("get primary property: %v", err)
	}
	if property.Value != "community-license" {
		t.Fatalf("primary property after request timeout = %q, want community-license", property.Value)
	}
}

func TestSetLicenseKeyReplacesDefinitivelyRejectedLegacyLicense(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			if r.Header.Get("Authorization") == "License invalid-license" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"errors":[{"title":"License Invalid","detail":"license rejected","code":"LICENSE_INVALID"}]}`)
				return
			}
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(CommunityEntitlement, EnterpriseEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := t.Context()
	gatewayClient := newTestLicenseGatewayClient(t)
	if _, err := gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, "invalid-license"); err != nil {
		t.Fatalf("seed invalid legacy license: %v", err)
	}
	provider, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	if err := provider.SetLicenseKey(ctx, "enterprise-license"); err != nil {
		t.Fatalf("SetLicenseKey(): %v", err)
	}
	property, err := gatewayClient.GetProperty(ctx, LicenseKeyPropertyKey)
	if err != nil {
		t.Fatalf("get primary property: %v", err)
	}
	if property.Value != "enterprise-license" {
		t.Fatalf("primary property = %q, want enterprise-license", property.Value)
	}
}

func TestSetLicenseKeyDoesNotRestoreLegacyCommunityLicenseDeletedByAnotherReplica(t *testing.T) {
	communityLookupStarted := make(chan struct{}, 1)
	releaseCommunityLookup := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			if r.Header.Get("Authorization") == "License community-license" {
				select {
				case communityLookupStarted <- struct{}{}:
				default:
				}
				<-releaseCommunityLookup
			}
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			if r.Header.Get("Authorization") == "License community-license" {
				_, _ = fmt.Fprint(w, entitlementsResponse(CommunityEntitlement))
				return
			}
			_, _ = fmt.Fprint(w, entitlementsResponse(CommunityEntitlement, EnterpriseEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := t.Context()
	gatewayClient := newTestLicenseGatewayClient(t)
	setter, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("create setter provider: %v", err)
	}
	remover, err := newProvider(ctx, gatewayClient, Config{}, server.URL)
	if err != nil {
		t.Fatalf("create remover provider: %v", err)
	}
	if _, err := gatewayClient.SetProperty(ctx, LicenseKeyPropertyKey, "community-license"); err != nil {
		t.Fatalf("seed legacy Community license: %v", err)
	}

	setDone := make(chan error, 1)
	go func() {
		setDone <- setter.SetLicenseKey(ctx, "enterprise-license")
	}()
	select {
	case <-communityLookupStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for legacy Community classification")
	}

	if err := remover.RemoveLicenseKey(ctx); err != nil {
		t.Fatalf("remove legacy Community license: %v", err)
	}
	close(releaseCommunityLookup)
	if err := <-setDone; err != nil {
		t.Fatalf("SetLicenseKey(): %v", err)
	}

	primary, err := gatewayClient.GetProperty(ctx, LicenseKeyPropertyKey)
	if err != nil {
		t.Fatalf("get primary property: %v", err)
	}
	if primary.Value != "enterprise-license" {
		t.Fatalf("primary property = %q, want enterprise-license", primary.Value)
	}
	if _, err := gatewayClient.GetProperty(ctx, CommunityLicenseKeyPropertyKey); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("Community fallback error = %v, want record not found", err)
	}
}

func TestCachedSnapshotMatchesEquivalentTimestamps(t *testing.T) {
	updatedAt := time.Now()
	provider := &Provider{
		licenseKeySnapshot: licenseKeySnapshot{
			key:         "license-key",
			updatedAt:   updatedAt,
			propertyKey: LicenseKeyPropertyKey,
		},
	}

	// Database round trips strip time.Time's monotonic clock reading.
	databaseSnapshot := licenseKeySnapshot{
		key:         "license-key",
		updatedAt:   updatedAt.Round(0),
		propertyKey: LicenseKeyPropertyKey,
	}
	if !provider.cachedSnapshotMatches(databaseSnapshot) {
		t.Fatal("expected snapshots representing the same timestamp to match")
	}
	databaseSnapshot.propertyKey = CommunityLicenseKeyPropertyKey
	if provider.cachedSnapshotMatches(databaseSnapshot) {
		t.Fatal("expected snapshots from different properties not to match")
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
