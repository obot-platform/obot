package billing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

type fakeStore struct {
	lock       sync.Mutex
	properties map[string]string
	getErr     error
	setErr     error
	sets       int
}

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newFakeStore() *fakeStore {
	return &fakeStore{properties: map[string]string{}}
}

func (f *fakeStore) GetProperty(_ context.Context, key string) (gatewaytypes.Property, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.getErr != nil {
		return gatewaytypes.Property{}, f.getErr
	}
	value, ok := f.properties[key]
	if !ok {
		return gatewaytypes.Property{}, gorm.ErrRecordNotFound
	}
	return gatewaytypes.Property{
		Key:   key,
		Value: value,
	}, nil
}

func (f *fakeStore) SetProperty(_ context.Context, key, value string) (gatewaytypes.Property, error) {
	f.lock.Lock()
	defer f.lock.Unlock()

	if f.setErr != nil {
		return gatewaytypes.Property{}, f.setErr
	}
	f.properties[key] = value
	f.sets++
	return gatewaytypes.Property{
		Key:   key,
		Value: value,
	}, nil
}

func testConfig() Config {
	return Config{
		BillingURL: "https://billing.obot.ai",
		BillingKey: "billing-key",
	}
}

func jsonResponse(t *testing.T, status int, body any) *http.Response {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("encoding response: %v", err)
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(string(encoded))),
	}
}

func TestConfigConfigured(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   bool
	}{
		{
			name:   "neither setting",
			config: Config{},
		},
		{
			name:   "url only",
			config: Config{BillingURL: "https://billing.obot.ai"},
		},
		{
			name:   "key only",
			config: Config{BillingKey: "billing-key"},
		},
		{
			name:   "blank settings",
			config: Config{BillingURL: "  ", BillingKey: "  "},
		},
		{
			name:   "both settings",
			config: testConfig(),
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.Configured(); got != tt.want {
				t.Fatalf("Configured() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestNewRequiresBillingSettings(t *testing.T) {
	if _, err := New(t.Context(), Config{}, newFakeStore()); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("New() error = %v, want %v", err, ErrNotConfigured)
	}
}

func TestRefreshReadsAndPersistsEntitlements(t *testing.T) {
	store := newFakeStore()
	client, err := New(t.Context(), testConfig(), store)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	var gotURL, gotAuthorization string
	client.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		gotURL = req.URL.String()
		gotAuthorization = req.Header.Get("Authorization")
		return jsonResponse(t, http.StatusOK, Entitlements{
			Plan:                  "Business",
			Status:                StatusActive,
			Seats:                 20,
			HostedMCPServers:      15,
			AuditLogRetentionDays: 30,
		}), nil
	})}

	if err := client.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh() error = %v, want nil", err)
	}

	if want := "https://billing.obot.ai/v1/entitlements"; gotURL != want {
		t.Fatalf("requested %q, want %q", gotURL, want)
	}
	if want := "Bearer billing-key"; gotAuthorization != want {
		t.Fatalf("Authorization = %q, want %q", gotAuthorization, want)
	}

	entitlements := client.Entitlements()
	if entitlements == nil || entitlements.Seats != 20 || entitlements.Plan != "Business" {
		t.Fatalf("Entitlements() = %+v, want the fetched response", entitlements)
	}
	if store.properties[EntitlementsPropertyKey] == "" {
		t.Fatal("entitlements were not persisted")
	}
}

func TestRefreshFailureKeepsTheLastGoodEntitlements(t *testing.T) {
	store := newFakeStore()
	client, err := New(t.Context(), testConfig(), store)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	var fail bool
	client.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		if fail {
			return nil, errors.New("billing service is down")
		}
		return jsonResponse(t, http.StatusOK, Entitlements{
			Status: StatusActive,
			Seats:  20,
		}), nil
	})}

	if err := client.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh() error = %v, want nil", err)
	}

	fail = true
	if err := client.Refresh(t.Context()); err == nil {
		t.Fatal("Refresh() error = nil, want a failure")
	}

	entitlements := client.Entitlements()
	if entitlements == nil || entitlements.Seats != 20 {
		t.Fatalf("Entitlements() = %+v, want the last good response", entitlements)
	}
}

func TestRefreshRejectsANonOKStatus(t *testing.T) {
	store := newFakeStore()
	client, err := New(t.Context(), testConfig(), store)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	client.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(t, http.StatusUnauthorized, map[string]string{"error": "unauthorized"}), nil
	})}

	if err := client.Refresh(t.Context()); err == nil {
		t.Fatal("Refresh() error = nil, want a failure")
	}
	if client.Entitlements() != nil {
		t.Fatalf("Entitlements() = %+v, want nil", client.Entitlements())
	}
	if store.sets != 0 {
		t.Fatalf("persisted %d times, want 0", store.sets)
	}
}

func TestEntitlementsSurviveARestart(t *testing.T) {
	store := newFakeStore()
	client, err := New(t.Context(), testConfig(), store)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	client.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(t, http.StatusOK, Entitlements{
			Plan:   "Team",
			Status: StatusTrialing,
			Seats:  10,
		}), nil
	})}
	if err := client.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh() error = %v, want nil", err)
	}

	// A restart while the billing service is unreachable.
	restarted, err := New(t.Context(), testConfig(), store)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	restarted.httpClient = &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("billing service is down")
	})}

	entitlements := restarted.Entitlements()
	if entitlements == nil || entitlements.Seats != 10 || entitlements.Plan != "Team" {
		t.Fatalf("Entitlements() = %+v, want the persisted response", entitlements)
	}
	if err := restarted.Refresh(t.Context()); err == nil {
		t.Fatal("Refresh() error = nil, want a failure")
	}
	if got := restarted.Entitlements(); got == nil || got.Seats != 10 {
		t.Fatalf("Entitlements() = %+v, want the persisted response", got)
	}
}

func TestUnreadablePersistedEntitlementsDoNotFailStartup(t *testing.T) {
	store := newFakeStore()
	store.properties[EntitlementsPropertyKey] = "not json"

	client, err := New(t.Context(), testConfig(), store)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if client.Entitlements() != nil {
		t.Fatalf("Entitlements() = %+v, want nil", client.Entitlements())
	}
}
