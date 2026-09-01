package producttelemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
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
	maxRetryDelay            = 5 * time.Minute
)

// Client sends product telemetry reports to Upgrade Server.
type Client struct {
	httpClient     *http.Client
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
		retryDelay:     retryDelay,
		sleep:          sleep,
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
		retry, retryAfter, err := c.send(ctx, body)
		if err == nil {
			return nil
		}
		if !retry || attempt == c.maxAttempts-1 {
			return err
		}
		delay := c.retryDelay(attempt)
		if retryAfter != nil {
			delay = *retryAfter
		}
		if err := c.sleep(ctx, delay); err != nil {
			return fmt.Errorf("send product telemetry request: %w", err)
		}
	}

	return nil
}

func (c *Client) send(ctx context.Context, body []byte) (bool, *time.Duration, error) {
	requestContext, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, c.requestURL, bytes.NewReader(body))
	if err != nil {
		return false, nil, fmt.Errorf("create product telemetry request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return false, nil, fmt.Errorf("send product telemetry request: %w", ctx.Err())
		}
		return true, nil, fmt.Errorf("send product telemetry request: %w", err)
	}
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		_ = response.Body.Close()
		return false, nil, nil
	}

	retry := response.StatusCode == http.StatusTooManyRequests ||
		response.StatusCode >= http.StatusInternalServerError && response.StatusCode <= 599
	var retryAfter *time.Duration
	if retry {
		if delay, ok := parseRetryAfter(response.Header.Get("Retry-After"), time.Now()); ok {
			retryAfter = &delay
		}
	}
	return retry, retryAfter, fmt.Errorf("product telemetry request failed: %w", apiclient.ErrorFromResponse(response))
}

// retryDelay applies equal jitter to exponential backoff: half of the delay is
// fixed and the other half is randomized to keep clients from retrying in sync.
func retryDelay(attempt int) time.Duration {
	delay := initialRetryDelay << attempt
	return delay/2 + time.Duration(rand.Int64N(int64(delay/2)+1))
}

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}

	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
		if seconds < 0 {
			return 0, false
		}
		if seconds > int64(maxRetryDelay/time.Second) {
			return maxRetryDelay, true
		}
		return time.Duration(seconds) * time.Second, true
	}

	retryAt, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	return min(max(retryAt.Sub(now), 0), maxRetryDelay), true
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
