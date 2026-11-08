// Package mcpserverrequest alerts administrators about users' requests to add MCP servers.
package mcpserverrequest

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

// Notifier posts new requests to a Slack incoming webhook. A Notifier without a webhook
// does nothing.
type Notifier struct {
	webhookURL string
	baseURL    string
	client     *http.Client
}

func NewNotifier(webhookURL, baseURL string) *Notifier {
	return &Notifier{
		webhookURL: strings.TrimSpace(webhookURL),
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

// Enabled reports whether requests are sent anywhere.
func (n *Notifier) Enabled() bool {
	return n != nil && n.webhookURL != ""
}

// ReviewURL is the page where an administrator approves or denies the request.
func (n *Notifier) ReviewURL(id string) string {
	if n == nil {
		return "/mcp-server-requests/" + id
	}
	return n.baseURL + "/mcp-server-requests/" + id
}

// NotifyRequested alerts administrators that a request needs a decision.
func (n *Notifier) NotifyRequested(ctx context.Context, request types.MCPServerRequest) error {
	if !n.Enabled() {
		return nil
	}
	return n.post(ctx, slackMessage(request, n.ReviewURL(request.ID)))
}

func (n *Notifier) post(ctx context.Context, message map[string]any) error {
	body, err := json.Marshal(message)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("post to Slack: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("post to Slack: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

// slackEscape escapes the characters Slack's mrkdwn treats as control characters.
func slackEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

func requestTarget(m types.MCPServerRequestManifest) string {
	switch m.Runtime {
	case types.RuntimeRemote:
		return m.RemoteURL
	case types.RuntimeNPX, types.RuntimeUVX:
		return fmt.Sprintf("%s %s", m.Runtime, m.Package)
	case types.RuntimeContainerized:
		return m.Image
	}
	return string(m.Runtime)
}

func slackMessage(r types.MCPServerRequest, reviewURL string) map[string]any {
	requester := r.RequesterName
	if r.RequesterEmail != "" && r.RequesterEmail != requester {
		requester = fmt.Sprintf("%s (%s)", cmpOr(requester, r.RequesterEmail), r.RequesterEmail)
	}
	requester = cmpOr(requester, r.RequesterEmail, r.RequesterID)

	fields := []map[string]any{
		{"type": "mrkdwn", "text": "*Server*\n" + slackEscape(r.Name)},
		{"type": "mrkdwn", "text": "*Requested by*\n" + slackEscape(requester)},
		{"type": "mrkdwn", "text": "*Runs as*\n`" + slackEscape(requestTarget(r.MCPServerRequestManifest)) + "`"},
	}
	if r.SourceURL != "" {
		fields = append(fields, map[string]any{"type": "mrkdwn", "text": "*Source*\n" + slackEscape(r.SourceURL)})
	}
	if id := cmpOr(r.ExistingCatalogEntryID, r.MatchedCatalogEntryID); id != "" {
		fields = append(fields, map[string]any{"type": "mrkdwn", "text": "*Already in the catalog*\n`" + slackEscape(id) + "`"})
	}

	blocks := []map[string]any{
		{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": fmt.Sprintf(":inbox_tray: *New MCP server request:* %s", slackEscape(r.Name))}},
		{"type": "section", "fields": fields},
	}
	if r.Reason != "" {
		blocks = append(blocks, map[string]any{"type": "section", "text": map[string]any{"type": "mrkdwn", "text": "*Why*\n>" + strings.ReplaceAll(slackEscape(r.Reason), "\n", "\n>")}})
	}
	blocks = append(blocks, map[string]any{
		"type": "actions",
		"elements": []map[string]any{{
			"type":  "button",
			"style": "primary",
			"text":  map[string]any{"type": "plain_text", "text": "Review request"},
			"url":   reviewURL,
		}},
	})
	return map[string]any{
		// The text is the notification's fallback and preview.
		"text":   fmt.Sprintf("New MCP server request from %s: %s — review it at %s", requester, r.Name, reviewURL),
		"blocks": blocks,
	}
}

func cmpOr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
