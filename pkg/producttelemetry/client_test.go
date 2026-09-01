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
		InstallationID: "7d7d83d8-2af0-4da8-ae2d-102d8eaa70be",
		ReportedAt:     time.Date(2026, time.August, 31, 0, 4, 12, 0, time.UTC),
		Metrics: clienttypes.ProductTelemetryMetrics{
			TotalUsers:        new(int64(42)),
			ActiveUsers:       new(int64(10)),
			BuiltInMCPServers: &servers,
		},
	}
}

func TestClientSend(t *testing.T) {
	wantBody := `{"installationID":"7d7d83d8-2af0-4da8-ae2d-102d8eaa70be","reportedAt":"2026-08-31T00:04:12Z","metrics":{"totalUsers":42,"activeUsers":10,"deployedMCPServers":null,"builtInMCPServers":[{"id":"github","name":"GitHub","deploymentCount":2,"userCount":7}],"authProviderType":null,"mcpAuditLogCount":null,"llmAuditLogCount":null}}`

	for _, responseBody := range []string{"", "ignored success body"} {
		t.Run(responseBody, func(t *testing.T) {
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
				if string(body) != wantBody {
					t.Errorf("body = %s, want %s", body, wantBody)
				}
				response.WriteHeader(http.StatusAccepted)
				_, _ = io.WriteString(response, responseBody)
			}))
			defer server.Close()

			client := NewClient(server.URL+"/root/", server.Client())
			if err := client.Send(t.Context(), testRequest()); err != nil {
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
