// Package billing reads an Obot Cloud environment's subscription and entitlements from obot-billing. It's on only when
// a billing key is configured.
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

var (
	// entitlementFields are the fields billing always sends. Answers missing any are refused, since a missing limit would
	// read as unlimited.
	entitlementFields = []string{
		"enterpriseAuthProviders",
		"enterpriseModelProviders",
		"users",
		"devices",
		"hostedMcpServers",
		"memoryMiB",
		"cpuMillicores",
		"auditLogRetentionDays",
	}
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
	Code    string `json:"code"`
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

// Subscription reads the environment's subscription and entitlements. It refuses answers with an unknown status, a
// missing entitlement, or a negative limit.
func (c *Client) Subscription(ctx context.Context) (types.Subscription, types.Entitlements, error) {
	var answer json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/subscription", nil, http.StatusOK, &answer); err != nil {
		return types.Subscription{}, types.Entitlements{}, err
	}

	// Also decode by field name, since a missing limit and a null one decode the same.
	var subscription struct {
		types.Subscription
		Entitlements types.Entitlements `json:"entitlements"`
	}
	var fields struct {
		Entitlements map[string]json.RawMessage `json:"entitlements"`
	}
	if err := json.Unmarshal(answer, &subscription); err != nil {
		return types.Subscription{}, types.Entitlements{}, fmt.Errorf("failed to decode billing's subscription: %w", err)
	}
	if err := json.Unmarshal(answer, &fields); err != nil {
		return types.Subscription{}, types.Entitlements{}, fmt.Errorf("failed to decode billing's entitlements: %w", err)
	}
	switch subscription.Status {
	case types.SubscriptionStatusTrialing, types.SubscriptionStatusActive, types.SubscriptionStatusPastDue, types.SubscriptionStatusEnded:
	default:
		return types.Subscription{}, types.Entitlements{}, fmt.Errorf("billing answered with an unknown subscription status %q", subscription.Status)
	}
	for _, field := range entitlementFields {
		if _, ok := fields.Entitlements[field]; !ok {
			return types.Subscription{}, types.Entitlements{}, fmt.Errorf("billing's entitlements leave out %s", field)
		}
	}

	entitlements := subscription.Entitlements
	for field, limit := range map[string]*int64{
		"users":            entitlements.Users,
		"devices":          entitlements.Devices,
		"hostedMcpServers": entitlements.HostedMCPServers,
		"memoryMiB":        entitlements.MemoryMiB,
		"cpuMillicores":    entitlements.CPUMillicores,
	} {
		if limit != nil && *limit < 0 {
			return types.Subscription{}, types.Entitlements{}, fmt.Errorf("billing answered with a negative %s limit", field)
		}
	}
	return subscription.Subscription, entitlements, nil
}

// CreateSession opens the billing pages for an owner and returns a one-time link. Billing sends the owner back to
// returnURL when they're done.
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
