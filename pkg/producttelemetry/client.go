package producttelemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/obot-platform/obot/apiclient"
	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/upgrade"
)

const (
	productTelemetryEndpoint = "product-telemetry"
	requestTimeout           = 30 * time.Second
	maxRequestAttempts       = 5
	initialRetryDelay        = time.Second
)

type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Client sends product telemetry reports to Upgrade Server.
type Client struct {
	httpClient     httpClient
	requestURL     string
	requestTimeout time.Duration
	maxAttempts    int
	retryDelay     func(int) time.Duration
	sleep          func(context.Context, time.Duration) error
}

// NewClient creates a product telemetry client for the supplied Upgrade Server base URL.
func NewClient(baseURL string, client *http.Client) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{
		httpClient:     client,
		requestURL:     upgrade.EndpointURL(baseURL, productTelemetryEndpoint),
		requestTimeout: requestTimeout,
		maxAttempts:    maxRequestAttempts,
		retryDelay: func(attempt int) time.Duration {
			return initialRetryDelay << attempt
		},
		sleep: sleep,
	}
}

// Send submits a product telemetry request. The request is encoded once so every retry sends
// the same installation ID, reported time, and metrics.
func (c *Client) Send(ctx context.Context, request clienttypes.ProductTelemetryRequest) error {
	body, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal product telemetry request: %w", err)
	}

	for attempt := range c.maxAttempts {
		retry, err := c.send(ctx, body)
		if err == nil {
			return nil
		}
		if !retry || attempt == c.maxAttempts-1 {
			return err
		}
		if err := c.sleep(ctx, c.retryDelay(attempt)); err != nil {
			return fmt.Errorf("send product telemetry request: %w", err)
		}
	}

	return nil
}

func (c *Client) send(ctx context.Context, body []byte) (bool, error) {
	requestContext, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, c.requestURL, bytes.NewReader(body))
	if err != nil {
		return false, fmt.Errorf("create product telemetry request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return false, fmt.Errorf("send product telemetry request: %w", ctx.Err())
		}
		return true, fmt.Errorf("send product telemetry request: %w", err)
	}
	if response.StatusCode == http.StatusAccepted {
		_ = response.Body.Close()
		return false, nil
	}

	retry := response.StatusCode == http.StatusTooManyRequests ||
		response.StatusCode >= http.StatusInternalServerError && response.StatusCode <= 599
	return retry, fmt.Errorf("product telemetry request failed: %w", apiclient.ErrorFromResponse(response))
}

func sleep(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
