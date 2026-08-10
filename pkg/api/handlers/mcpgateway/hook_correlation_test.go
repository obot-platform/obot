package mcpgateway

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestMCPHookCorrelationStoreSharedAcrossProcessors(t *testing.T) {
	client := newMCPProxyTestStorage()
	metadata := map[string]string{"mcpID": "mcp-1", "userID": "user-1"}
	writer := newHookCorrelationStore(client, metadata)
	reader := newHookCorrelationStore(client, metadata)
	request := pendingRequest{
		message: mcp.Message{Method: "tools/call"},
		name:    "echo",
		mutations: map[string]mcp.HookMutation{
			"request": {Mutated: true, Reasons: []string{"redacted"}},
		},
	}

	if err := writer.save(t.Context(), "session-1", `"request-1"`, hookOriginClient, request); err != nil {
		t.Fatal(err)
	}
	stored, found, err := reader.loadAndDelete(t.Context(), "session-1", `"request-1"`, hookOriginClient)
	if err != nil {
		t.Fatal(err)
	}
	if !found || stored.message.Method != "tools/call" || stored.name != "echo" {
		t.Fatalf("unexpected correlation: found=%v request=%#v", found, stored)
	}
	if mutation := stored.mutations["request"]; !mutation.Mutated || len(mutation.Reasons) != 1 || mutation.Reasons[0] != "redacted" {
		t.Fatalf("unexpected stored mutation: %#v", mutation)
	}
	if _, found, err := reader.loadAndDelete(t.Context(), "session-1", `"request-1"`, hookOriginClient); err != nil || found {
		t.Fatalf("correlation was not consumed: found=%v err=%v", found, err)
	}

	var correlations v1.MCPHookCorrelationList
	if err := client.List(t.Context(), &correlations, kclient.InNamespace("default")); err != nil {
		t.Fatal(err)
	}
	if len(correlations.Items) != 0 {
		t.Fatalf("got %d remaining correlations, want none", len(correlations.Items))
	}
}

func TestMCPHookCorrelationStoreSeparatesOrigins(t *testing.T) {
	client := newMCPProxyTestStorage()
	store := newHookCorrelationStore(client, map[string]string{"mcpID": "mcp-1", "userID": "user-1"})
	clientRequest := pendingRequest{message: mcp.Message{Method: "tools/call"}}
	serverRequest := pendingRequest{message: mcp.Message{Method: "roots/list"}}

	if err := store.save(t.Context(), "session-1", "1", hookOriginClient, clientRequest); err != nil {
		t.Fatal(err)
	}
	if err := store.save(t.Context(), "session-1", "1", hookOriginServer, serverRequest); err != nil {
		t.Fatal(err)
	}
	got, found, err := store.loadAndDelete(t.Context(), "session-1", "1", hookOriginServer)
	if err != nil || !found || got.message.Method != "roots/list" {
		t.Fatalf("server correlation: found=%v method=%q err=%v", found, got.message.Method, err)
	}
	got, found, err = store.loadAndDelete(t.Context(), "session-1", "1", hookOriginClient)
	if err != nil || !found || got.message.Method != "tools/call" {
		t.Fatalf("client correlation: found=%v method=%q err=%v", found, got.message.Method, err)
	}
}

func TestMCPHookCorrelationStoreReusesKey(t *testing.T) {
	client := newMCPProxyTestStorage()
	store := newHookCorrelationStore(client, map[string]string{"mcpID": "mcp-1", "userID": "user-1"})

	if err := store.save(t.Context(), "session-1", "1", hookOriginClient,
		pendingRequest{message: mcp.Message{Method: "tools/list"}}); err != nil {
		t.Fatal(err)
	}
	key := store.key("session-1", "1", hookOriginClient)
	var first v1.MCPHookCorrelation
	if err := client.Get(t.Context(), key, &first); err != nil {
		t.Fatal(err)
	}
	if err := store.save(t.Context(), "session-1", "1", hookOriginClient,
		pendingRequest{message: mcp.Message{Method: "tools/call"}, name: "echo"}); err != nil {
		t.Fatal(err)
	}
	var reused v1.MCPHookCorrelation
	if err := client.Get(t.Context(), key, &reused); err != nil {
		t.Fatal(err)
	}
	if reused.Spec.ExpiresAt.Before(&first.Spec.ExpiresAt) {
		t.Fatalf("reused correlation expiry moved backwards: first=%v reused=%v", first.Spec.ExpiresAt, reused.Spec.ExpiresAt)
	}

	got, found, err := store.loadAndDelete(t.Context(), "session-1", "1", hookOriginClient)
	if err != nil || !found || got.message.Method != "tools/call" || got.name != "echo" {
		t.Fatalf("reused correlation: found=%v request=%#v err=%v", found, got, err)
	}
}

func TestMCPHookCorrelationStoreRejectsExpiredCorrelation(t *testing.T) {
	client := newMCPProxyTestStorage()
	store := newHookCorrelationStore(client, map[string]string{"mcpID": "mcp-1", "userID": "user-1"})

	if err := store.save(t.Context(), "session-1", "1", hookOriginClient,
		pendingRequest{message: mcp.Message{Method: "tools/call"}, name: "echo"}); err != nil {
		t.Fatal(err)
	}
	key := store.key("session-1", "1", hookOriginClient)
	var correlation v1.MCPHookCorrelation
	if err := client.Get(t.Context(), key, &correlation); err != nil {
		t.Fatal(err)
	}
	correlation.Spec.ExpiresAt = metav1.NewTime(time.Now().Add(-time.Minute))
	if err := client.Update(t.Context(), &correlation); err != nil {
		t.Fatal(err)
	}

	if got, found, err := store.loadAndDelete(t.Context(), "session-1", "1", hookOriginClient); err != nil || found {
		t.Fatalf("expired correlation was returned: found=%v request=%#v err=%v", found, got, err)
	}

	var correlations v1.MCPHookCorrelationList
	if err := client.List(t.Context(), &correlations, kclient.InNamespace("default")); err != nil {
		t.Fatal(err)
	}
	if len(correlations.Items) != 0 {
		t.Fatalf("got %d remaining correlations, want none", len(correlations.Items))
	}
}
