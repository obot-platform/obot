package billing

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
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
		"hostedMcpServers": 3,
		"memoryMiB": 4096,
		"cpuMillicores": 2000,
		"auditLogRetentionDays": 7
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

	subscription, entitlements, err := NewClient(server.URL+"/", " billing-key\n").Subscription(t.Context())
	if err != nil {
		t.Fatalf("Subscription() error = %v", err)
	}
	if subscription.Plan != "Team" || subscription.Status != types.SubscriptionStatusTrialing || subscription.PeriodEnd == nil ||
		subscription.CancelAtPeriodEnd || !subscription.SelfService {
		t.Fatalf("subscription = %+v", subscription)
	}
	if !entitlements.EnterpriseAuthProviders || entitlements.EnterpriseModelProviders || entitlements.Devices != nil ||
		*entitlements.Users != 10 || *entitlements.HostedMCPServers != 3 || *entitlements.MemoryMiB != 4096 ||
		*entitlements.CPUMillicores != 2000 || *entitlements.AuditLogRetentionDays != 7 {
		t.Fatalf("entitlements = %+v", entitlements)
	}
}

func TestClientSubscriptionRefusesIncompleteAnswers(t *testing.T) {
	for _, tt := range []struct {
		name, body string
	}{
		{
			name: "missing entitlement",
			body: strings.Replace(subscriptionJSON, `"hostedMcpServers": 3,`, ``, 1),
		},
		{
			name: "unknown status",
			body: strings.Replace(subscriptionJSON, `"trialing"`, `"paused"`, 1),
		},
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
			if _, _, err := NewClient(server.URL, "billing-key").Subscription(t.Context()); err == nil {
				t.Fatal("Subscription() error = nil")
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
		_, _ = w.Write([]byte(`{"url": "https://billing.obot.ai/manage/one-time", "expiresAt": "2026-10-20T12:05:00Z"}`))
	}))
	defer server.Close()

	session, err := NewClient(server.URL, "billing-key").CreateSession(t.Context(), "https://acme.on-obot.ai/admin/platform?billing=returned")
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if session.URL != "https://billing.obot.ai/manage/one-time" || session.ExpiresAt.Time.IsZero() {
		t.Fatalf("session = %+v", session)
	}
}

func TestClientErrors(t *testing.T) {
	for _, tt := range []struct {
		name        string
		status      int
		body        string
		wantCode    string
		wantMessage string
	}{
		{
			name:        "billing error",
			status:      http.StatusConflict,
			body:        `{"error": {"code": "not_self_service", "message": "This plan has no billing pages."}}`,
			wantCode:    "not_self_service",
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
			if billingErr.Status != tt.status || billingErr.Code != tt.wantCode || billingErr.Message != tt.wantMessage {
				t.Fatalf("error = %+v", billingErr)
			}
		})
	}
}
