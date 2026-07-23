package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

type testPropertyClient struct {
	lock  sync.Mutex
	value string
	calls int
	err   error
}

func (c *testPropertyClient) GetOrCreateProperty(_ context.Context, key, value string) (gatewaytypes.Property, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.calls++
	if c.err != nil {
		return gatewaytypes.Property{}, c.err
	}
	if c.value == "" {
		c.value = value
	}
	return gatewaytypes.Property{Key: key, Value: c.value}, nil
}

func TestServerBaseURLAndEndpointURL(t *testing.T) {
	t.Setenv("OBOT_UPGRADE_SERVER_URL", " https://upgrade.example.test/root/ ")
	if got := ServerBaseURL(); got != "https://upgrade.example.test/root/" {
		t.Fatalf("ServerBaseURL() = %q", got)
	}
	if got := EndpointURL(ServerBaseURL(), "/community-license"); got != "https://upgrade.example.test/root/community-license" {
		t.Fatalf("EndpointURL() = %q", got)
	}

	t.Setenv("OBOT_UPGRADE_SERVER_URL", "   ")
	if got := ServerBaseURL(); got != DefaultServerBaseURL {
		t.Fatalf("ServerBaseURL() = %q, want default", got)
	}
}

func TestCommunityLicenseIssue(t *testing.T) {
	const (
		issuer         = "https://obot.example.test"
		installationID = "stable-installation-id"
		licenseKey     = "keygen/community-secret-key"
	)

	propertyClient := &testPropertyClient{value: installationID}
	var requests []CommunityLicenseRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.Method)
		}
		if r.URL.Path != "/root/community-license" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}

		var request CommunityLicenseRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		requests = append(requests, request)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"licenseKey":%q}`, licenseKey)
	}))
	defer server.Close()

	client := NewCommunityLicenseIssuer(propertyClient, server.URL+"/root/", server.Client())
	for _, request := range []CommunityLicenseRequest{
		{Name: "  Ada Lovelace  ", Email: " ada@example.com ", Company: "   "},
		{Name: "Grace Hopper", Email: "grace@example.com", Company: "  US Navy  "},
	} {
		got, err := client.Issue(t.Context(), request)
		if err != nil {
			t.Fatalf("Issue() error = %v", err)
		}
		if got != licenseKey {
			t.Fatalf("Issue() = %q", got)
		}
	}

	if propertyClient.calls != 2 {
		t.Fatalf("installation property calls = %d", propertyClient.calls)
	}
	if len(requests) != 2 {
		t.Fatalf("request count = %d", len(requests))
	}
	if requests[0] != (CommunityLicenseRequest{Name: "Ada Lovelace", Email: "ada@example.com", InstallationID: installationID}) {
		t.Fatalf("first request = %#v", requests[0])
	}
	if requests[1].Company != "US Navy" || requests[1].InstallationID != installationID {
		t.Fatalf("second request = %#v", requests[1])
	}
}

func TestCommunityLicenseIssueFailuresAreSanitized(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantError  error
	}{
		{
			name:       "non-200",
			statusCode: http.StatusBadRequest,
			body:       `upstream secret response`,
			wantError:  ErrCommunityLicenseRequest,
		},
		{
			name:       "malformed JSON",
			statusCode: http.StatusOK,
			body:       `{"licenseKey":`,
			wantError:  ErrCommunityLicenseResponse,
		},
		{
			name:       "unknown response field",
			statusCode: http.StatusOK,
			body:       `{"licenseKey":"key","secret":"upstream secret response"}`,
			wantError:  ErrCommunityLicenseResponse,
		},
		{
			name:       "trailing response",
			statusCode: http.StatusOK,
			body:       `{"licenseKey":"key"} {"other":true}`,
			wantError:  ErrCommunityLicenseResponse,
		},
		{
			name:       "blank key",
			statusCode: http.StatusOK,
			body:       `{"licenseKey":"   "}`,
			wantError:  ErrCommunityLicenseResponse,
		},
		{
			name:       "oversized response",
			statusCode: http.StatusOK,
			body:       `{"licenseKey":"` + strings.Repeat("x", maxCommunityLicenseResponseBytes) + `"}`,
			wantError:  ErrCommunityLicenseResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			client := NewCommunityLicenseIssuer(&testPropertyClient{value: "installation"}, server.URL, server.Client())
			_, err := client.Issue(t.Context(), CommunityLicenseRequest{Name: "Sensitive Name", Email: "secret@example.com"})
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("Issue() error = %v, want %v", err, tt.wantError)
			}
			for _, sensitive := range []string{"upstream secret response", "Sensitive Name", "secret@example.com"} {
				if strings.Contains(err.Error(), sensitive) {
					t.Fatalf("error leaked %q: %v", sensitive, err)
				}
			}
		})
	}
}

func TestCommunityLicenseIssueCancellationAndTimeout(t *testing.T) {
	releaseServer := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-releaseServer:
		}
	}))
	defer server.Close()
	defer close(releaseServer)

	client := NewCommunityLicenseIssuer(&testPropertyClient{value: "installation"}, server.URL, server.Client())

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Issue(cancelled, CommunityLicenseRequest{}); !errors.Is(err, context.Canceled) || !errors.Is(err, ErrCommunityLicenseRequest) {
		t.Fatalf("cancelled Issue() error = %v", err)
	}

	timedOut, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.Issue(timedOut, CommunityLicenseRequest{}); !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrCommunityLicenseRequest) {
		t.Fatalf("timed out Issue() error = %v", err)
	}
}

func TestCommunityLicenseIssueDependencyFailuresAreSanitized(t *testing.T) {
	secret := errors.New("sensitive dependency details")
	propertyClient := &testPropertyClient{err: secret}
	client := NewCommunityLicenseIssuer(propertyClient, "https://upgrade.test", nil)
	if _, err := client.Issue(t.Context(), CommunityLicenseRequest{}); !errors.Is(err, ErrCommunityLicenseRequest) || strings.Contains(err.Error(), secret.Error()) {
		t.Fatalf("property error was not sanitized: %v", err)
	}

	client = NewCommunityLicenseIssuer(&testPropertyClient{value: "installation"}, "https://upgrade.test", nil)
	if _, err := client.Issue(t.Context(), CommunityLicenseRequest{}); !errors.Is(err, ErrCommunityLicenseRequest) || strings.Contains(err.Error(), secret.Error()) {
		t.Fatalf("token error was not sanitized: %v", err)
	}
}
