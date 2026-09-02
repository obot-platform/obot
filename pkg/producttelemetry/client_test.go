package producttelemetry

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testRequest() clienttypes.ProductTelemetryRequest {
	servers := []clienttypes.ProductTelemetryBuiltInMCPServer{{
		ID:              "github",
		Name:            "GitHub",
		DeploymentCount: 2,
		UserCount:       7,
	}}
	return clienttypes.ProductTelemetryRequest{
		InstallationID:   "7d7d83d8-2af0-4da8-ae2d-102d8eaa70be",
		LicenseMachineID: "ab65c9ac-c012-4567-89ab-1b520aa26584",
		ReportedAt:       time.Date(2026, time.August, 31, 0, 4, 12, 0, time.UTC),
		Distribution:     clienttypes.ProductTelemetryDistributionCloud,
		Engine:           "kubernetes",
		CurrentVersion:   "v0.26.0",
		Metrics: &clienttypes.ProductTelemetryMetrics{
			TotalUsers:                  new(int64(42)),
			DeployedMCPServers:          new(int64(0)),
			CustomMCPServerEntryCount:   new(int64(4)),
			BuiltInMCPServers:           &servers,
			AuthProviderType:            new("github"),
			MCPAuditLogCount:            new(int64(0)),
			SentryScanCount:             new(int64(14)),
			SentryEnforcementEventCount: new(int64(3)),
			ManagedSkillCount:           new(int64(27)),
		},
	}
}

func TestClientSend(t *testing.T) {
	fullRequest := testRequest()
	metadataOnlyRequest := testRequest()
	metadataOnlyRequest.Metrics = nil

	tests := []struct {
		name         string
		request      clienttypes.ProductTelemetryRequest
		wantBody     string
		responseBody string
	}{
		{
			name:         "full report preserves null and zero metrics",
			request:      fullRequest,
			wantBody:     `{"installationID":"7d7d83d8-2af0-4da8-ae2d-102d8eaa70be","licenseMachineID":"ab65c9ac-c012-4567-89ab-1b520aa26584","reportedAt":"2026-08-31T00:04:12Z","distribution":"Cloud","engine":"kubernetes","currentVersion":"v0.26.0","metrics":{"totalUsers":42,"activeUsers":null,"deployedMCPServers":0,"customMCPServerEntryCount":4,"builtInMCPServers":[{"id":"github","name":"GitHub","deploymentCount":2,"userCount":7}],"authProviderType":"github","mcpAuditLogCount":0,"llmAuditLogCount":null,"sentryScanCount":14,"sentryEnforcementEventCount":3,"managedSkillCount":27}}`,
			responseBody: "ignored success body",
		},
		{
			name:     "metadata-only report omits metrics",
			request:  metadataOnlyRequest,
			wantBody: `{"installationID":"7d7d83d8-2af0-4da8-ae2d-102d8eaa70be","licenseMachineID":"ab65c9ac-c012-4567-89ab-1b520aa26584","reportedAt":"2026-08-31T00:04:12Z","distribution":"Cloud","engine":"kubernetes","currentVersion":"v0.26.0"}`,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				if request.Method != http.MethodPost {
					t.Errorf("method = %q, want POST", request.Method)
				}
				if request.URL.Path != "/root/product-telemetry" {
					t.Errorf("path = %q", request.URL.Path)
				}
				if got := request.Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", got)
				}
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
				if string(body) != testCase.wantBody {
					t.Errorf("body = %s, want %s", body, testCase.wantBody)
				}
				response.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(response, testCase.responseBody)
			}))
			defer server.Close()

			client := NewClient(server.URL+"/root/", server.Client())
			if err := client.Send(t.Context(), testCase.request); err != nil {
				t.Fatalf("Send() error = %v", err)
			}
		})
	}
}

func TestClientDoesNotRetryNonRetryableResponse(t *testing.T) {
	for _, statusCode := range []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType} {
		t.Run(http.StatusText(statusCode), func(t *testing.T) {
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				http.Error(response, "specific server error", statusCode)
			}))
			defer server.Close()

			err := NewClient(server.URL, server.Client()).Send(t.Context(), testRequest())
			var httpErr *clienttypes.ErrHTTP
			if !errors.As(err, &httpErr) || httpErr.Code != statusCode {
				t.Fatalf("Send() error = %v, want HTTP status %d", err, statusCode)
			}
			if !strings.Contains(err.Error(), "specific server error") {
				t.Fatalf("Send() error = %v, want response body", err)
			}
			if got := attempts.Load(); got != 1 {
				t.Fatalf("attempts = %d, want 1", got)
			}
		})
	}
}

func TestClientRetriesTransientFailures(t *testing.T) {
	tests := []struct {
		name       string
		firstRound roundTripFunc
	}{
		{
			name: "transport error",
			firstRound: func(*http.Request) (*http.Response, error) {
				return nil, errors.New("temporary network failure")
			},
		},
		{
			name: "rate limited",
			firstRound: func(*http.Request) (*http.Response, error) {
				return httpResponse(http.StatusTooManyRequests, "try later"), nil
			},
		},
		{
			name: "server error",
			firstRound: func(*http.Request) (*http.Response, error) {
				return httpResponse(http.StatusBadGateway, "try later"), nil
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var attempts int
			var bodies [][]byte
			transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(request.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				bodies = append(bodies, body)
				attempts++
				if attempts < 3 {
					return testCase.firstRound(request)
				}
				return httpResponse(http.StatusAccepted, ""), nil
			})
			client := NewClient("https://upgrade.example.test", &http.Client{Transport: transport})
			var delays []time.Duration
			client.retryDelay = exponentialRetryDelay
			client.sleep = func(_ context.Context, delay time.Duration) error {
				delays = append(delays, delay)
				return nil
			}

			if err := client.Send(t.Context(), testRequest()); err != nil {
				t.Fatalf("Send() error = %v", err)
			}
			if attempts != 3 {
				t.Fatalf("attempts = %d, want 3", attempts)
			}
			if len(delays) != 2 || delays[0] != time.Second || delays[1] != 2*time.Second {
				t.Fatalf("delays = %v, want [1s 2s]", delays)
			}
			for index := 1; index < len(bodies); index++ {
				if !bytes.Equal(bodies[0], bodies[index]) {
					t.Fatalf("retry body %d changed", index+1)
				}
			}
		})
	}
}

func TestClientRetryExhaustion(t *testing.T) {
	t.Run("response", func(t *testing.T) {
		var attempts int
		transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts++
			return httpResponse(http.StatusServiceUnavailable, "still unavailable"), nil
		})
		client := NewClient("https://upgrade.example.test", &http.Client{Transport: transport})
		var delays []time.Duration
		client.retryDelay = exponentialRetryDelay
		client.sleep = func(_ context.Context, delay time.Duration) error {
			delays = append(delays, delay)
			return nil
		}

		err := client.Send(t.Context(), testRequest())
		var httpErr *clienttypes.ErrHTTP
		if !errors.As(err, &httpErr) || httpErr.Code != http.StatusServiceUnavailable ||
			!strings.Contains(err.Error(), "still unavailable") {
			t.Fatalf("Send() error = %v, want final status and response body", err)
		}
		if attempts != maxRequestAttempts {
			t.Fatalf("attempts = %d, want %d", attempts, maxRequestAttempts)
		}
		wantDelays := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
		if len(delays) != len(wantDelays) {
			t.Fatalf("delays = %v, want %v", delays, wantDelays)
		}
		for index := range wantDelays {
			if delays[index] != wantDelays[index] {
				t.Fatalf("delays = %v, want %v", delays, wantDelays)
			}
		}
	})

	t.Run("transport", func(t *testing.T) {
		wantErr := errors.New("network unavailable")
		var attempts int
		transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
			attempts++
			return nil, wantErr
		})
		client := NewClient("https://upgrade.example.test", &http.Client{Transport: transport})
		client.sleep = func(context.Context, time.Duration) error { return nil }

		err := client.Send(t.Context(), testRequest())
		if !errors.Is(err, wantErr) {
			t.Fatalf("Send() error = %v, want %v", err, wantErr)
		}
		if attempts != maxRequestAttempts {
			t.Fatalf("attempts = %d, want %d", attempts, maxRequestAttempts)
		}
	})
}

func TestClientHonorsRetryAfter(t *testing.T) {
	var attempts int
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			response := httpResponse(http.StatusTooManyRequests, "rate limited")
			response.Header.Set("Retry-After", "60")
			return response, nil
		}
		return httpResponse(http.StatusAccepted, ""), nil
	})
	client := NewClient("https://upgrade.example.test", &http.Client{Transport: transport})
	client.retryDelay = func(int) time.Duration { return time.Millisecond }
	var delays []time.Duration
	client.sleep = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}

	if err := client.Send(t.Context(), testRequest()); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if len(delays) != 1 || delays[0] != time.Minute {
		t.Fatalf("delays = %v, want [1m0s]", delays)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		value     string
		wantDelay time.Duration
		wantOK    bool
	}{
		{name: "seconds", value: " 120 ", wantDelay: 2 * time.Minute, wantOK: true},
		{name: "seconds clamped", value: "999999999", wantDelay: maxRetryDelay, wantOK: true},
		{name: "zero seconds", value: "0", wantOK: true},
		{name: "HTTP date", value: now.Add(3 * time.Minute).Format(http.TimeFormat), wantDelay: 3 * time.Minute, wantOK: true},
		{name: "HTTP date clamped", value: now.Add(24 * time.Hour).Format(http.TimeFormat), wantDelay: maxRetryDelay, wantOK: true},
		{name: "past HTTP date", value: now.Add(-time.Minute).Format(http.TimeFormat), wantOK: true},
		{name: "negative seconds", value: "-1"},
		{name: "invalid", value: "later"},
		{name: "empty"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			delay, ok := parseRetryAfter(testCase.value, now)
			if ok != testCase.wantOK || delay != testCase.wantDelay {
				t.Fatalf("parseRetryAfter(%q) = (%v, %v), want (%v, %v)", testCase.value, delay, ok, testCase.wantDelay, testCase.wantOK)
			}
		})
	}
}

func TestClientReturnsBoundedHTTPError(t *testing.T) {
	body := strings.Repeat("x", 100_000) + "secret tail"
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return httpResponse(http.StatusBadRequest, body), nil
	})
	client := NewClient("https://upgrade.example.test", &http.Client{Transport: transport})

	err := client.Send(t.Context(), testRequest())
	var httpErr *clienttypes.ErrHTTP
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest || !strings.HasSuffix(httpErr.Message, "…") {
		t.Fatalf("Send() error = %v, want bounded HTTP error", err)
	}
	if strings.Contains(err.Error(), "secret tail") {
		t.Fatalf("Send() error included body beyond limit")
	}
}

func TestClientCancellationStopsRequestAndBackoff(t *testing.T) {
	t.Run("active request", func(t *testing.T) {
		started := make(chan struct{})
		transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
			close(started)
			<-request.Context().Done()
			return nil, request.Context().Err()
		})
		client := NewClient("https://upgrade.example.test", &http.Client{Transport: transport})
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- client.Send(ctx, testRequest()) }()
		<-started
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("Send() error = %v, want context canceled", err)
		}
	})

	t.Run("backoff", func(t *testing.T) {
		attempted := make(chan struct{})
		transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
			close(attempted)
			return httpResponse(http.StatusServiceUnavailable, "retry"), nil
		})
		client := NewClient("https://upgrade.example.test", &http.Client{Transport: transport})
		client.retryDelay = func(int) time.Duration { return time.Hour }
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- client.Send(ctx, testRequest()) }()
		<-attempted
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("Send() error = %v, want context canceled", err)
		}
	})
}

func TestClientAttemptTimeout(t *testing.T) {
	var attempts atomic.Int32
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		attempts.Add(1)
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	client := NewClient("https://upgrade.example.test", &http.Client{Transport: transport})
	client.requestTimeout = 20 * time.Millisecond
	client.maxAttempts = 1

	err := client.Send(t.Context(), testRequest())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Send() error = %v, want deadline exceeded", err)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

func TestClientMarshalFailureDoesNotSend(t *testing.T) {
	var attempts atomic.Int32
	transport := roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)
		return httpResponse(http.StatusAccepted, ""), nil
	})
	client := NewClient("https://upgrade.example.test", &http.Client{Transport: transport})
	request := testRequest()
	request.ReportedAt = time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC)

	if err := client.Send(t.Context(), request); err == nil || !strings.Contains(err.Error(), "marshal product telemetry request") {
		t.Fatalf("Send() error = %v, want marshal error", err)
	}
	if got := attempts.Load(); got != 0 {
		t.Fatalf("attempts = %d, want 0", got)
	}
}

func httpResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func exponentialRetryDelay(attempt int) time.Duration {
	return initialRetryDelay << attempt
}
