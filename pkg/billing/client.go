package billing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	requestTimeout = 15 * time.Second
	// maxResponseBytes is the largest body billing sends.
	maxResponseBytes = 64 << 10
)

// Client calls billing's API for Obot with the environment's billing key.
type Client struct {
	url  string
	key  string
	http *http.Client
}

// Error is an error response from billing. Message is safe to show to owners.
type Error struct {
	Status  int    `json:"-"`
	Message string `json:"message"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("billing responded %d: %s", e.Status, e.Message)
}

func NewClient(url, key string) *Client {
	return &Client{
		url:  strings.TrimSuffix(url, "/"),
		key:  strings.TrimSpace(key),
		http: &http.Client{Timeout: requestTimeout},
	}
}

// FetchSubscription reads the environment's subscription, entitlements included. It refuses an answer with a
// negative limit, which nothing could enforce.
func (c *Client) FetchSubscription(ctx context.Context) (types.Subscription, error) {
	var subscription types.Subscription
	if err := c.do(ctx, http.MethodGet, "/api/v1/subscription", nil, http.StatusOK, &subscription); err != nil {
		return types.Subscription{}, err
	}

	granted := subscription.Entitlements
	for _, limit := range []struct {
		field string
		value *int64
	}{
		{"users", granted.Users},
		{"devices", granted.Devices},
		{"memoryMiB", granted.MemoryMiB},
		{"cpuMillicores", granted.CPUMillicores},
		{"auditLogRetentionDays", granted.AuditLogRetentionDays},
	} {
		if limit.value != nil && *limit.value < 0 {
			return types.Subscription{}, fmt.Errorf("billing answered with a negative %s", limit.field)
		}
	}
	return subscription, nil
}

// CreateSession opens the billing pages for an owner and returns a one-time link to them. Billing sends the owner
// back to returnURL when they're done.
func (c *Client) CreateSession(ctx context.Context, returnURL string) (types.BillingSession, error) {
	var session types.BillingSession
	return session, c.do(ctx, http.MethodPost, "/api/v1/sessions", map[string]string{"returnUrl": returnURL}, http.StatusCreated, &session)
}

// do sends a request to billing and decodes the response into out. Any other status than wantStatus is an *Error.
func (c *Client) do(ctx context.Context, method, path string, in any, wantStatus int, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.url+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach billing: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return fmt.Errorf("failed to read billing's answer: %w", err)
	}
	if resp.StatusCode != wantStatus {
		// Fall back to the status text when there's no error body.
		var answer struct {
			Error Error `json:"error"`
		}
		_ = json.Unmarshal(data, &answer)
		answer.Error.Status = resp.StatusCode
		if answer.Error.Message == "" {
			answer.Error.Message = http.StatusText(resp.StatusCode)
		}
		return &answer.Error
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("failed to decode billing's answer: %w", err)
	}
	return nil
}
