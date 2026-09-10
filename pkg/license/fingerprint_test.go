package license

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMachineFingerprintUsesPersistedInstallationIdentity(t *testing.T) {
	client := newTestLicenseGatewayClient(t)
	if _, err := client.SetProperty(t.Context(), LicenseMachineIDPropertyKey, "existing-installation"); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		provider, err := NewProvider(t.Context(), client, Config{})
		if err != nil {
			t.Fatal(err)
		}

		if provider.MachineFingerprint() != "existing-installation" {
			t.Fatal("replaced persisted machine fingerprint")
		}
	}
}

func TestHasValidLicenseWithoutEntitlements(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse())
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse())
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse())
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	provider, err := newProvider(t.Context(), nil, Config{LicenseKey: "valid-license"}, upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	if !requireValidLicense(t.Context(), t, provider) {
		t.Fatal("valid license without entitlements was rejected")
	}
}
