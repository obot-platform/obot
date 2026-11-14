package billing

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	subscriptionJSON = `{
	"plan": "Team",
	"status": "trialing",
	"periodEnd": "2026-10-22T00:00:00Z",
	"cancelAtPeriodEnd": false,
	"paymentRequired": false,
	"selfService": true,
	"entitlements": {
		"enterpriseAuthProviders": true,
		"enterpriseModelProviders": false,
		"users": 10,
		"devices": null,
		"memoryMiB": 4096,
		"cpuMillicores": 2000,
		"auditLogRetentionDays": 7,
		"auditLogStorageMiB": 10240,
		"networkEgressMiB": 30720
	}
}`
)

func serve(t *testing.T, method, path string, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method || r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer billing-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestClientSubscription(t *testing.T) {
	server := serve(t, http.MethodGet, "/api/v1/subscription", http.StatusOK, subscriptionJSON)

	subscription, err := NewClient(server.URL+"/", " billing-key\n").FetchSubscription(t.Context())
	if err != nil {
		t.Fatalf("FetchSubscription() error = %v", err)
	}
	if subscription.Plan != "Team" || subscription.Status != "trialing" || subscription.PeriodEnd == nil ||
		subscription.CancelAtPeriodEnd || !subscription.SelfService {
		t.Fatalf("subscription = %+v", subscription)
	}
	if entitlements := subscription.Entitlements; !entitlements.EnterpriseAuthProviders || entitlements.EnterpriseModelProviders || entitlements.Devices != nil ||
		*entitlements.Users != 10 || *entitlements.MemoryMiB != 4096 ||
		*entitlements.CPUMillicores != 2000 || *entitlements.AuditLogRetentionDays != 7 ||
		*entitlements.AuditLogStorageMiB != 10240 || *entitlements.NetworkEgressMiB != 30720 {
		t.Fatalf("entitlements = %+v", entitlements)
	}
}

func TestClientSubscriptionRefusesBadAnswers(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
	}{
		{
			name: "negative limit",
			body: strings.Replace(subscriptionJSON, `"users": 10`, `"users": -1`, 1),
		},
		{
			name: "not JSON",
			body: `<html>`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := serve(t, http.MethodGet, "/api/v1/subscription", http.StatusOK, tt.body)
			if _, err := NewClient(server.URL, "billing-key").FetchSubscription(t.Context()); err == nil {
				t.Fatal("FetchSubscription() error = nil")
			}
		})
	}
}

func TestClientCreateSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["returnUrl"] != "https://acme.on-obot.ai/admin/platform?billing=returned" {
			t.Errorf("body = %v, %v", body, err)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/sessions" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"url": "https://billing.obot.ai/manage/one-time"}`))
	}))
	defer server.Close()

	session, err := NewClient(server.URL, "billing-key").CreateSession(t.Context(), "https://acme.on-obot.ai/admin/platform?billing=returned")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if session.URL != "https://billing.obot.ai/manage/one-time" {
		t.Fatalf("session = %+v", session)
	}
}

func TestClientErrors(t *testing.T) {
	for _, tt := range []struct {
		name        string
		status      int
		body        string
		wantMessage string
	}{
		{
			name:        "billing error",
			status:      http.StatusConflict,
			body:        `{"error": {"code": "not_self_service", "message": "This plan has no billing pages."}}`,
			wantMessage: "This plan has no billing pages.",
		},
		{
			name:        "no error body",
			status:      http.StatusServiceUnavailable,
			wantMessage: http.StatusText(http.StatusServiceUnavailable),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := serve(t, http.MethodPost, "/api/v1/sessions", tt.status, tt.body)
			_, err := NewClient(server.URL, "billing-key").CreateSession(t.Context(), "https://acme.on-obot.ai")
			billingErr, ok := errors.AsType[*Error](err)
			if !ok {
				t.Fatalf("CreateSession() error = %v, want *Error", err)
			}
			if billingErr.Status != tt.status || billingErr.Message != tt.wantMessage {
				t.Fatalf("error = %+v", billingErr)
			}
		})
	}
}
