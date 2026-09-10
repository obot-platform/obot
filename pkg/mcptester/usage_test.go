package mcptester

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestModelProxyUsageEndpointAndRedirects(t *testing.T) {
	for _, tc := range []struct {
		base string
		want string
	}{
		{
			base: "https://proxy.example",
			want: "https://proxy.example/v1/usage",
		},
		{
			base: "https://proxy.example/prefix/",
			want: "https://proxy.example/prefix/v1/usage",
		},
		{
			base: "https://proxy.example/prefix/v1/responses/",
			want: "https://proxy.example/prefix/v1/usage",
		},
		{
			base: "https://proxy.example/a%2Fb/v1/responses",
			want: "https://proxy.example/a%2Fb/v1/usage",
		},
		{
			base: "https://proxy.example/a%2Fb/%76%31/%72esponses",
			want: "https://proxy.example/a%2Fb/v1/usage",
		},
	} {
		endpoint, err := ParseModelProxyURL(tc.base, false)
		if err != nil {
			t.Fatal(err)
		}

		r, err := NewModelProxyUsageRequest(t.Context(), endpoint, "key", "machine")
		if err != nil || r.URL.String() != tc.want || r.Method != http.MethodGet || r.Header.Get("Accept") != "application/json" {
			t.Fatalf("usage request = %v, %v", r, err)
		}

		if endpoint.String() == tc.want {
			t.Fatal("usage builder mutated generation URL")
		}
	}

	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("redirect target received installation credentials")
	}))
	defer target.Close()

	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()

	endpoint, err := ParseModelProxyURL(redirect.URL, true)
	if err != nil {
		t.Fatal(err)
	}

	r, err := NewModelProxyUsageRequest(t.Context(), endpoint, "key", "machine")
	if err != nil {
		t.Fatal(err)
	}

	client := NewModelProxyUsageHTTPClient()
	if client.Timeout != ModelProxyUsageTimeout {
		t.Fatal("incorrect usage timeout")
	}

	resp, err := client.Do(r)
	if err != nil {
		t.Fatal(err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatal("redirect followed")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := client.Do(r.WithContext(ctx)); !errors.Is(err, context.Canceled) {
		t.Fatalf("request did not honor cancellation: %v", err)
	}
}

func TestReadModelProxyUsage(t *testing.T) {
	const valid = `{"input":{"used":100,"max":0},"output":{"used":20,"max":10},"reset_at":"2026-09-11T00:00:00Z"}`

	for _, tc := range []struct {
		name       string
		status     int
		body       string
		retryAfter string
		wantStatus int
		wantRetry  string
	}{
		{
			name:   "valid including exceeded maxima",
			status: http.StatusOK,
			body:   valid,
		},
		{
			name:       "missing counters",
			status:     http.StatusOK,
			body:       `{"input":{"used":100},"output":{"used":20,"max":10},"reset_at":"2026-09-11T00:00:00Z"}`,
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "null object",
			status:     http.StatusOK,
			body:       "null",
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "negative usage",
			status:     http.StatusOK,
			body:       strings.Replace(valid, "100", "-1", 1),
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "oversized body",
			status:     http.StatusOK,
			body:       valid + strings.Repeat(" ", 64*1024),
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "multiple documents",
			status:     http.StatusOK,
			body:       valid + valid,
			wantStatus: http.StatusBadGateway,
		},
		{
			name:       "invalid license",
			status:     http.StatusForbidden,
			body:       "private upstream error",
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "missing license",
			status:     http.StatusUnauthorized,
			body:       "private upstream error",
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "rate limited",
			status:     http.StatusTooManyRequests,
			body:       "private upstream error",
			retryAfter: "60",
			wantStatus: http.StatusTooManyRequests,
			wantRetry:  "60",
		},
		{
			name:       "unavailable cooldown",
			status:     http.StatusServiceUnavailable,
			retryAfter: "300",
			wantStatus: http.StatusServiceUnavailable,
			wantRetry:  "300",
		},
		{
			name:       "unsafe retry header",
			status:     http.StatusTooManyRequests,
			retryAfter: "private upstream details",
			wantStatus: http.StatusTooManyRequests,
		},
		{
			name:       "unsupported proxy endpoint",
			status:     http.StatusNotFound,
			wantStatus: http.StatusBadGateway,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}
			response.Header.Set("Retry-After", tc.retryAfter)
			got, err := ReadModelProxyUsage(response)
			if tc.wantStatus == 0 {
				if err != nil || got.Input.Used != 100 || got.Input.Max != 0 || got.Output.Used != 20 || got.Output.Max != 10 {
					t.Fatalf("usage = %+v, %v", got, err)
				}

				return
			}

			failure, ok := errors.AsType[*ModelProxyUsageError](err)
			if !ok || failure.Status != tc.wantStatus || failure.RetryAfter != tc.wantRetry || strings.Contains(failure.Message, "private") {
				t.Fatalf("unsafe or incorrect error: %v", err)
			}
		})
	}
}
