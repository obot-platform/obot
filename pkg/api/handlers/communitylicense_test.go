package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/upgrade"
	"k8s.io/apiserver/pkg/authentication/user"
)

type fakeCommunityLicenseProvider struct {
	lock              sync.Mutex
	key               string
	configured        bool
	valid             bool
	entitlements      []string
	licenseKeyError   error
	validErrors       []error
	entitlementsError error
	setError          error
	setCalls          int
	hasValidCalls     int
	lastInstalledKey  string
	validateError     error
}

func (p *fakeCommunityLicenseProvider) LicenseKey(context.Context) (string, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.key, p.licenseKeyError
}

func (p *fakeCommunityLicenseProvider) LicenseKeyViaConfiguration() bool {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.configured
}

func (p *fakeCommunityLicenseProvider) SetLicenseKey(_ context.Context, key string) error {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.setCalls++
	p.lastInstalledKey = key
	if p.setError != nil {
		return p.setError
	}
	p.key = key
	p.valid = true
	p.entitlements = []string{license.CommunityEntitlement}
	return nil
}

func (p *fakeCommunityLicenseProvider) RemoveLicenseKey(context.Context) error {
	return nil
}

func (p *fakeCommunityLicenseProvider) Validate(context.Context) error {
	return p.validateError
}

func (p *fakeCommunityLicenseProvider) HasValidLicense(context.Context) (bool, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	call := p.hasValidCalls
	p.hasValidCalls++
	if call < len(p.validErrors) && p.validErrors[call] != nil {
		return false, p.validErrors[call]
	}
	return p.valid, nil
}

func (p *fakeCommunityLicenseProvider) Entitlements(context.Context) ([]string, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	return append([]string(nil), p.entitlements...), p.entitlementsError
}

type fakeCommunityIssuer struct {
	lock     sync.Mutex
	key      string
	errors   []error
	requests []upgrade.CommunityLicenseRequest
	started  chan struct{}
	release  chan struct{}
}

func (i *fakeCommunityIssuer) Issue(ctx context.Context, request upgrade.CommunityLicenseRequest) (string, error) {
	i.lock.Lock()
	i.requests = append(i.requests, request)
	call := len(i.requests) - 1
	var err error
	if call < len(i.errors) {
		err = i.errors[call]
	}
	started := i.started
	release := i.release
	key := i.key
	i.lock.Unlock()

	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if err != nil {
		return "", err
	}
	return key, nil
}

func (i *fakeCommunityIssuer) requestCount() int {
	i.lock.Lock()
	defer i.lock.Unlock()
	return len(i.requests)
}

func (i *fakeCommunityIssuer) request(index int) upgrade.CommunityLicenseRequest {
	i.lock.Lock()
	defer i.lock.Unlock()
	return i.requests[index]
}

func communityAPIContext(t *testing.T, body string) (api.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/license/community", strings.NewReader(body))
	return api.Context{
		ResponseWriter: recorder,
		Request:        request,
		User: &user.DefaultInfo{
			Name:   "admin",
			Groups: apitypes.RoleAdmin.Groups(),
		},
	}, recorder
}

func requireHTTPError(t *testing.T, err error, code int) {
	t.Helper()
	var httpError *apitypes.ErrHTTP
	if !errors.As(err, &httpError) {
		t.Fatalf("expected HTTP error, got %T: %v", err, err)
	}
	if httpError.Code != code {
		t.Fatalf("HTTP status = %d, want %d: %v", httpError.Code, code, err)
	}
}

func TestCreateCommunityLicenseValidatesAndNormalizesInput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "malformed input",
			body: `{"name":"Sensitive Name"`,
		},
		{
			name: "missing name",
			body: `{"email":"ada@example.com"}`,
		},
		{
			name: "whitespace name",
			body: `{"name":"   ","email":"ada@example.com"}`,
		},
		{
			name: "missing email",
			body: `{"name":"Ada"}`,
		},
		{
			name: "whitespace email",
			body: `{"name":"Ada","email":"   "}`},
		{
			name: "invalid email",
			body: `{"name":"Ada","email":"not-an-email"}`,
		},
		{
			name: "email without domain suffix",
			body: `{"name":"Ada","email":"s@t"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeCommunityLicenseProvider{}
			issuer := &fakeCommunityIssuer{key: "issued-key"}
			handler := NewLicenseHandler(provider, issuer)
			request, _ := communityAPIContext(t, tt.body)
			err := handler.CreateCommunityLicense(request)
			requireHTTPError(t, err, http.StatusBadRequest)
			if issuer.requestCount() != 0 {
				t.Fatalf("issuer called %d times", issuer.requestCount())
			}
			if strings.Contains(err.Error(), "Sensitive Name") {
				t.Fatalf("error leaked submitted data: %v", err)
			}
		})
	}

	provider := &fakeCommunityLicenseProvider{}
	issuer := &fakeCommunityIssuer{key: "keygen/community-12345678"}
	handler := NewLicenseHandler(provider, issuer)
	request, _ := communityAPIContext(t, `{"name":"  Ada Lovelace  ","email":" Ada <ADA@example.com> ","company":"   "}`)
	if err := handler.CreateCommunityLicense(request); err != nil {
		t.Fatalf("CreateCommunityLicense() error = %v", err)
	}
	got := issuer.request(0)
	if got.Name != "Ada Lovelace" || got.Email != "ADA@example.com" || got.Company != "" {
		t.Fatalf("normalized request = %#v", got)
	}
	if got.InstallationID != "" {
		t.Fatalf("browser handler supplied installation ID %q", got.InstallationID)
	}
}

func TestCreateCommunityLicenseEligibility(t *testing.T) {
	tests := []struct {
		name     string
		provider *fakeCommunityLicenseProvider
	}{
		{name: "valid license", provider: &fakeCommunityLicenseProvider{key: "existing", valid: true}},
		{name: "configured invalid license", provider: &fakeCommunityLicenseProvider{key: "configured", configured: true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issuer := &fakeCommunityIssuer{key: "issued-key"}
			handler := NewLicenseHandler(tt.provider, issuer)
			request, _ := communityAPIContext(t, `{"name":"Ada","email":"ada@example.com"}`)
			requireHTTPError(t, handler.CreateCommunityLicense(request), http.StatusConflict)
			if issuer.requestCount() != 0 {
				t.Fatal("issuer was called for an ineligible installation")
			}
		})
	}
}

func TestCreateCommunityLicensePropagatesEligibilityLookupError(t *testing.T) {
	lookupErr := errors.New("license lookup failed")
	tests := []struct {
		name        string
		validErrors []error
	}{
		{name: "initial check", validErrors: []error{lookupErr}},
		{name: "check after reservation", validErrors: []error{nil, lookupErr}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeCommunityLicenseProvider{validErrors: tt.validErrors}
			issuer := &fakeCommunityIssuer{key: "issued-key"}
			handler := NewLicenseHandler(provider, issuer)
			request, recorder := communityAPIContext(t, `{"name":"Ada","email":"ada@example.com"}`)

			err := handler.CreateCommunityLicense(request)
			if !errors.Is(err, lookupErr) {
				t.Fatalf("CreateCommunityLicense() error = %v, want %v", err, lookupErr)
			}
			if issuer.requestCount() != 0 {
				t.Fatalf("issuer called %d times after eligibility lookup failed", issuer.requestCount())
			}
			if provider.setCalls != 0 {
				t.Fatalf("license installed %d times after eligibility lookup failed", provider.setCalls)
			}
			if recorder.Body.Len() != 0 {
				t.Fatalf("response written after eligibility lookup failed: %s", recorder.Body.String())
			}

			provider.validErrors = nil
			retryRequest, _ := communityAPIContext(t, `{"name":"Ada","email":"ada@example.com"}`)
			if err := handler.CreateCommunityLicense(retryRequest); err != nil {
				t.Fatalf("retry after eligibility lookup failed: %v", err)
			}
			if issuer.requestCount() != 1 {
				t.Fatalf("issuer called %d times after retry, want 1", issuer.requestCount())
			}
		})
	}
}

func TestCreateCommunityLicensePropagatesStatusLookupErrors(t *testing.T) {
	statusErr := errors.New("license status lookup failed")
	tests := []struct {
		name     string
		provider *fakeCommunityLicenseProvider
	}{
		{
			name:     "license key",
			provider: &fakeCommunityLicenseProvider{licenseKeyError: statusErr},
		},
		{
			name:     "valid license",
			provider: &fakeCommunityLicenseProvider{validErrors: []error{nil, nil, statusErr}},
		},
		{
			name:     "entitlements",
			provider: &fakeCommunityLicenseProvider{entitlementsError: statusErr},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issuer := &fakeCommunityIssuer{key: "keygen/community-12345678"}
			handler := NewLicenseHandler(tt.provider, issuer)
			request, recorder := communityAPIContext(t, `{"name":"Ada","email":"ada@example.com"}`)

			err := handler.CreateCommunityLicense(request)
			if !errors.Is(err, statusErr) {
				t.Fatalf("CreateCommunityLicense() error = %v, want %v", err, statusErr)
			}
			if issuer.requestCount() != 1 {
				t.Fatalf("issuer called %d times, want 1", issuer.requestCount())
			}
			if tt.provider.setCalls != 1 {
				t.Fatalf("license installed %d times, want 1", tt.provider.setCalls)
			}
			if recorder.Body.Len() != 0 {
				t.Fatalf("response written after status lookup failed: %s", recorder.Body.String())
			}
		})
	}
}

func TestCreateCommunityLicenseReplacesInvalidDatabaseKeyAndReturnsMaskedStatus(t *testing.T) {
	const issuedKey = "keygen/community-full-12345678"
	provider := &fakeCommunityLicenseProvider{key: "invalid-database-key"}
	issuer := &fakeCommunityIssuer{key: issuedKey}
	handler := NewLicenseHandler(provider, issuer)
	request, recorder := communityAPIContext(t, `{"name":"Ada","email":"ada@example.com","company":"Analytical Engines"}`)

	if err := handler.CreateCommunityLicense(request); err != nil {
		t.Fatalf("CreateCommunityLicense() error = %v", err)
	}
	if provider.lastInstalledKey != issuedKey || provider.setCalls != 1 {
		t.Fatalf("installed key = %q, calls = %d", provider.lastInstalledKey, provider.setCalls)
	}
	var status LicenseStatus
	if err := json.NewDecoder(recorder.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if status.LicenseKey != "****12345678" || status.Source != "database" || !status.Enterprise {
		t.Fatalf("status = %#v", status)
	}
	if len(status.Entitlements) != 1 || status.Entitlements[0] != license.CommunityEntitlement {
		t.Fatalf("entitlements = %v", status.Entitlements)
	}
	responseBody := recorder.Body.String()
	for _, sensitive := range []string{issuedKey, "Ada", "ada@example.com", "Analytical Engines"} {
		if strings.Contains(responseBody, sensitive) {
			t.Fatalf("response leaked %q: %s", sensitive, responseBody)
		}
	}
}

func TestCreateCommunityLicenseMapsIssuerAndInstallFailures(t *testing.T) {
	secret := "sensitive upstream response and issued key"
	tests := []struct {
		name       string
		issuerErr  error
		installErr error
		wantCode   int
	}{
		{
			name:      "issuer failure",
			issuerErr: errors.New(secret),
			wantCode:  http.StatusBadGateway,
		},
		{
			name:       "invalid issued key",
			installErr: license.ErrInvalidLicense,
			wantCode:   http.StatusBadGateway,
		},
		{
			name:       "configuration changes during issuance",
			installErr: license.ErrLicenseKeyViaConfiguration,
			wantCode:   http.StatusConflict,
		},
		{
			name:       "entitlement or persistence failure",
			installErr: errors.New(secret),
			wantCode:   http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &fakeCommunityLicenseProvider{setError: tt.installErr}
			issuer := &fakeCommunityIssuer{key: secret, errors: []error{tt.issuerErr}}
			handler := NewLicenseHandler(provider, issuer)
			request, _ := communityAPIContext(t, `{"name":"Sensitive Name","email":"secret@example.com"}`)
			err := handler.CreateCommunityLicense(request)
			requireHTTPError(t, err, tt.wantCode)
			for _, sensitive := range []string{secret, "Sensitive Name", "secret@example.com"} {
				if strings.Contains(err.Error(), sensitive) {
					t.Fatalf("error leaked %q: %v", sensitive, err)
				}
			}
		})
	}
}

func TestCreateCommunityLicenseSerializesRequestsAndAllowsRetry(t *testing.T) {
	provider := &fakeCommunityLicenseProvider{}
	issuer := &fakeCommunityIssuer{
		key:     "keygen/community-12345678",
		errors:  []error{errors.New("temporary failure")},
		started: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	handler := NewLicenseHandler(provider, issuer)
	body := `{"name":"Ada","email":"ada@example.com"}`

	firstRequest, _ := communityAPIContext(t, body)
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- handler.CreateCommunityLicense(firstRequest)
	}()
	<-issuer.started

	secondRequest, _ := communityAPIContext(t, body)
	requireHTTPError(t, handler.CreateCommunityLicense(secondRequest), http.StatusConflict)
	if issuer.requestCount() != 1 {
		t.Fatalf("issuer calls during concurrent request = %d", issuer.requestCount())
	}

	close(issuer.release)
	requireHTTPError(t, <-firstDone, http.StatusBadGateway)

	thirdRequest, recorder := communityAPIContext(t, body)
	if err := handler.CreateCommunityLicense(thirdRequest); err != nil {
		t.Fatalf("retry error = %v", err)
	}
	if issuer.requestCount() != 2 {
		t.Fatalf("issuer calls after retry = %d", issuer.requestCount())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte(`"enterprise":true`)) {
		t.Fatalf("retry response = %s", recorder.Body.String())
	}
}
