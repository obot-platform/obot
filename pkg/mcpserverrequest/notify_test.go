package mcpserverrequest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func TestNotifyRequestedPostsToSlack(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &got); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	n := NewNotifier(srv.URL, "https://obot.example.com/")
	err := n.NotifyRequested(context.Background(), types.MCPServerRequest{
		ID:            "msrq1abc",
		RequesterName: "Ada <script>",
		Name:          "Linear",
		Reason:        "Track issues\nfrom Claude",
		Runtime:       types.RuntimeRemote,
		RemoteURL:     "https://mcp.linear.app/mcp",
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(got)
	body := string(b)
	for _, want := range []string{
		"https://obot.example.com/mcp-server-requests/msrq1abc",
		"Ada \\u0026lt;script\\u0026gt;",
		"\\u003eTrack issues\\n\\u003efrom Claude",
		"https://mcp.linear.app/mcp",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("message missing %q:\n%s", want, body)
		}
	}
}

func TestNotifierDisabled(t *testing.T) {
	n := NewNotifier("", "https://obot.example.com")
	if n.Enabled() {
		t.Fatal("a notifier without a webhook must be disabled")
	}
	if err := n.NotifyRequested(context.Background(), types.MCPServerRequest{}); err != nil {
		t.Fatal(err)
	}
	var nilNotifier *Notifier
	if nilNotifier.Enabled() {
		t.Fatal("nil notifier must be disabled")
	}
}

func TestNotifyRequestedReportsSlackErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid_token", http.StatusForbidden)
	}))
	defer srv.Close()
	err := NewNotifier(srv.URL, "https://obot.example.com").NotifyRequested(context.Background(), types.MCPServerRequest{})
	if err == nil || !strings.Contains(err.Error(), "invalid_token") {
		t.Fatalf("err = %v", err)
	}
}
