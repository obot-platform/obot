package license

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	keygen "github.com/keygen-sh/keygen-go/v3"
)

func resetKeygen(t *testing.T) {
	t.Helper()

	account := keygen.Account
	product := keygen.Product
	licenseKey := keygen.LicenseKey
	token := keygen.Token
	publicKey := keygen.PublicKey
	apiURL := keygen.APIURL
	environment := keygen.Environment

	t.Cleanup(func() {
		keygen.Account = account
		keygen.Product = product
		keygen.LicenseKey = licenseKey
		keygen.Token = token
		keygen.PublicKey = publicKey
		keygen.APIURL = apiURL
		keygen.Environment = environment
	})
}

func TestRequireEntitlement(t *testing.T) {
	resetKeygen(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse("license-1"))
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse("license-1"))
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(EnterpriseAuthProvidersEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	keygen.APIURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider, err := NewProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	})
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	if !provider.HasValidLicense() {
		t.Fatal("expected license to be valid")
	}
	if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected entitlement to be accepted")
	}
	entitlements := provider.Entitlements()
	if len(entitlements) != 1 || entitlements[0] != EnterpriseAuthProvidersEntitlement {
		t.Fatalf("expected entitlement list to contain %q, got %v", EnterpriseAuthProvidersEntitlement, entitlements)
	}
}

func TestRequireEntitlementMissing(t *testing.T) {
	resetKeygen(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse("license-1"))
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse("license-1"))
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse("OTHER_ENTITLEMENT"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	keygen.APIURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider, err := NewProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	})
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	if !provider.HasValidLicense() {
		t.Fatal("expected license to be valid")
	}
	if provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected entitlement to be missing")
	}
}

func TestNewProviderNotConfigured(t *testing.T) {
	resetKeygen(t)

	provider, err := NewProvider(context.Background(), nil, Config{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider to be created")
	}
	if provider.HasValidLicense() {
		t.Fatal("expected license to be invalid")
	}
}

func TestNewProviderActivatesLicenseOnNoMachine(t *testing.T) {
	resetKeygen(t)

	machineFingerprint := ""
	validationCount := 0
	activated := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse("license-1"))
		case "/v1/licenses/license-1/actions/validate":
			validationCount++
			machineFingerprint = assertValidateFingerprint(t, r, machineFingerprint)
			if validationCount == 1 {
				_, _ = fmt.Fprint(w, validationResponseWithCode("license-1", "NO_MACHINE", false))
				return
			}
			_, _ = fmt.Fprint(w, validationResponse("license-1"))
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
	keygen.APIURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider, err := NewProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	})
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	if !activated {
		t.Fatal("expected license to be activated")
	}
	if validationCount != 2 {
		t.Fatalf("expected license to be validated before and after activation, got %d validations", validationCount)
	}
	if !provider.HasValidLicense() {
		t.Fatal("expected license to be valid after activation")
	}
	if !provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected entitlement to be accepted")
	}
}

func TestUpdateRefreshesEntitlements(t *testing.T) {
	resetKeygen(t)

	entitlement := EnterpriseAuthProvidersEntitlement
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch r.URL.Path {
		case "/v1/me":
			_, _ = fmt.Fprint(w, licenseResponse("license-1"))
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse("license-1"))
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(entitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	keygen.APIURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider, err := NewProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	})
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	entitlement = "OTHER_ENTITLEMENT"
	provider.update(ctx)

	if provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected old entitlement to be removed")
	}
	if !provider.hasEntitlement("OTHER_ENTITLEMENT") {
		t.Fatal("expected new entitlement to be added")
	}
}

func TestUpdateClearsEntitlementsWhenLicenseInvalid(t *testing.T) {
	resetKeygen(t)

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
			_, _ = fmt.Fprint(w, licenseResponse("license-1"))
		case "/v1/licenses/license-1/actions/validate":
			_, _ = fmt.Fprint(w, validationResponse("license-1"))
		case "/v1/licenses/license-1/entitlements":
			_, _ = fmt.Fprint(w, entitlementsResponse(EnterpriseAuthProvidersEntitlement))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	keygen.APIURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	provider, err := NewProvider(ctx, nil, Config{
		LicenseKey: "license-key",
	})
	if err != nil {
		t.Fatalf("expected provider to be created: %v", err)
	}

	invalid = true
	provider.update(ctx)

	if provider.HasValidLicense() {
		t.Fatal("expected license to be marked invalid")
	}
	if provider.hasEntitlement(EnterpriseAuthProvidersEntitlement) {
		t.Fatal("expected entitlement to be cleared")
	}
	if entitlements := provider.Entitlements(); len(entitlements) != 0 {
		t.Fatalf("expected entitlements to be cleared, got %v", entitlements)
	}
}

func licenseResponse(id string) string {
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
}`, id)
}

func validationResponse(id string) string {
	return validationResponseWithCode(id, "VALID", true)
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
	data := ""
	for i, code := range codes {
		if i > 0 {
			data += ","
		}
		data += fmt.Sprintf(`{
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
	return fmt.Sprintf(`{"data":[%s]}`, data)
}
