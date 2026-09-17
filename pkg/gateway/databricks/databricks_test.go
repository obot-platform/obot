package databricks

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type captureRoundTripper struct {
	req *http.Request
}

func (c *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.req = req
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func TestBaseURL(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		value   string
		wantErr bool
	}{
		{
			name:  "workspace",
			value: "https://example.cloud.databricks.com/",
		},
		{
			name:  "custom domain",
			value: "https://models.example.com",
		},
		{
			name:    "missing",
			wantErr: true,
		},
		{
			name:    "http",
			value:   "http://example.com",
			wantErr: true,
		},
		{
			name:    "path",
			value:   "https://example.com/path",
			wantErr: true,
		},
		{
			name:    "query",
			value:   "https://example.com?token=value",
			wantErr: true,
		},
		{
			name:    "userinfo",
			value:   "https://user@example.com",
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := BaseURL(map[string]string{WorkspaceURLEnv: test.value})
			if (err != nil) != test.wantErr {
				t.Fatalf("BaseURL() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestTransport(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		url      string
		wantAuth string
	}{
		{
			name:     "workspace",
			url:      "https://workspace.example/serving-endpoints/responses",
			wantAuth: "Bearer secret",
		},
		{
			name: "IPv4 discovery daemon",
			url:  "http://127.0.0.1:1234/v1/models",
		},
		{
			name: "IPv6 discovery daemon",
			url:  "http://[::1]:1234/v1/models",
		},
		{
			name: "localhost discovery daemon",
			url:  "http://localhost:1234/v1/models",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			capture := &captureRoundTripper{}
			transport, err := Transport(map[string]string{TokenEnv: "secret"}, capture)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, test.url, nil)
			req.Header.Set("Authorization", "Bearer obot-token")
			req.Header.Set("X-Api-Key", "obot-token")
			if _, err := transport.RoundTrip(req); err != nil {
				t.Fatal(err)
			}
			if got := capture.req.Header.Get("Authorization"); got != test.wantAuth {
				t.Errorf("Authorization = %q, want %q", got, test.wantAuth)
			}
			if got := capture.req.Header.Get("X-Api-Key"); got != "" {
				t.Errorf("X-Api-Key = %q, want empty", got)
			}
		})
	}
}

func TestTransportRequiresToken(t *testing.T) {
	t.Parallel()
	if _, err := Transport(nil, nil); err == nil {
		t.Fatal("Transport() error = nil, want error")
	}
}
