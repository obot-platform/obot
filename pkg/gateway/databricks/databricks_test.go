package databricks

import (
	"net/http"
	"net/http/httptest"
	"testing"

	llmtypes "github.com/obot-platform/obot/pkg/llm"
	"github.com/obot-platform/obot/pkg/system"
)

type captureRoundTripper struct {
	req *http.Request
}

type redirectRoundTripper struct {
	requests []*http.Request
}

func TestIsProvider(t *testing.T) {
	t.Parallel()
	if !IsProvider(system.DatabricksModelProvider) {
		t.Fatal("Databricks provider was not recognized")
	}
	if IsProvider(system.OpenAIModelProvider) {
		t.Fatal("OpenAI provider was recognized as Databricks")
	}
}

func TestResponsesPath(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name    string
		dialect llmtypes.Dialect
		want    string
		wantErr bool
	}{
		{
			name:    "OpenAI Responses",
			dialect: llmtypes.DialectOpenAIResponses,
			want:    "responses",
		},
		{
			name:    "Open Responses",
			dialect: llmtypes.DialectOpenResponses,
			want:    "open-responses",
		},
		{
			name:    "unsupported",
			dialect: llmtypes.DialectOpenAIChatCompletions,
			wantErr: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResponsesPath(test.dialect)
			if (err != nil) != test.wantErr {
				t.Fatalf("ResponsesPath() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Errorf("ResponsesPath() = %q, want %q", got, test.want)
			}
		})
	}
}

func (c *captureRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	c.req = req
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
}

func (r *redirectRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	captured := req.Clone(req.Context())
	captured.Header = req.Header.Clone()
	r.requests = append(r.requests, captured)
	if len(r.requests) == 1 {
		return &http.Response{
			StatusCode: http.StatusFound,
			Header: http.Header{
				"Location": []string{"https://redirect.example/target"},
			},
			Body:    http.NoBody,
			Request: req,
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       http.NoBody,
		Request:    req,
	}, nil
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

func TestBaseURLDefaultsToHTTPS(t *testing.T) {
	t.Parallel()

	got, err := BaseURL(map[string]string{
		WorkspaceURLEnv: "example.cloud.databricks.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "https://example.cloud.databricks.com" {
		t.Fatalf("BaseURL() = %q, want %q", got.String(), "https://example.cloud.databricks.com")
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
			name: "different host",
			url:  "https://redirect.example/serving-endpoints/responses",
		},
		{
			name: "different port",
			url:  "https://workspace.example:8443/serving-endpoints/responses",
		},
		{
			name: "HTTP downgrade",
			url:  "http://workspace.example/serving-endpoints/responses",
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
			transport, err := Transport(map[string]string{
				WorkspaceURLEnv: "https://workspace.example",
				TokenEnv:        "secret",
			}, capture)
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

func TestTransportDoesNotAuthenticateCrossOriginRedirect(t *testing.T) {
	t.Parallel()

	capture := &redirectRoundTripper{}
	transport, err := Transport(map[string]string{
		WorkspaceURLEnv: "https://workspace.example",
		TokenEnv:        "secret",
	}, capture)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := (&http.Client{Transport: transport}).Get("https://workspace.example/start")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	if len(capture.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(capture.requests))
	}
	if got := capture.requests[0].Header.Get("Authorization"); got != "Bearer secret" {
		t.Errorf("initial Authorization = %q, want bearer token", got)
	}
	if got := capture.requests[1].Header.Get("Authorization"); got != "" {
		t.Errorf("redirect Authorization = %q, want empty", got)
	}
}

func TestTransportRequiresToken(t *testing.T) {
	t.Parallel()
	if _, err := Transport(map[string]string{
		WorkspaceURLEnv: "https://workspace.example",
	}, nil); err == nil {
		t.Fatal("Transport() error = nil, want error")
	}
}
