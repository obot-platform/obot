package mcpconnect

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func wire(t *testing.T, value string) jsonrpc.Message {
	t.Helper()
	msg, err := jsonrpc.DecodeMessage([]byte(value))
	require.NoError(t, err)
	return msg
}

func TestBridgePreservesMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	requests := make(chan jsonrpc.Message, 10)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			return
		}
		var raw json.RawMessage
		require.NoError(t, json.NewDecoder(r.Body).Decode(&raw))
		msg, err := jsonrpc.DecodeMessage(raw)
		require.NoError(t, err)
		requests <- msg
		request, ok := msg.(*jsonrpc.Request)
		if !ok || !request.IsCall() {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Mcp-Session-Id", "session1")
		result := `{"ok":true}`
		if request.Method == "initialize" {
			require.Empty(t, r.Header.Get("MCP-Protocol-Version"))
			result = `{"protocolVersion":"2025-11-25","capabilities":{"tools":{"listChanged":true}},"serverInfo":{"name":"actual","version":"1"}}`
		} else {
			require.Equal(t, "2025-11-25", r.Header.Get("MCP-Protocol-Version"))
			require.Equal(t, "session1", r.Header.Get("Mcp-Session-Id"))
		}
		id, _ := json.Marshal(request.ID.Raw())
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":%s}`, id, result)
	}))
	defer gateway.Close()
	a, b := gomcp.NewInMemoryTransports()
	local, err := a.Connect(ctx)
	require.NoError(t, err)
	client, err := b.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()
	done := make(chan error, 1)
	go func() { done <- bridge(ctx, local, gateway.URL, nil, http.DefaultTransport) }()
	initialize := wire(t, `{"jsonrpc":"2.0","id":"init-original","method":"initialize","params":{"protocolVersion":"2099-01-01","clientInfo":{"name":"real client","version":"1"},"capabilities":{"roots":{"listChanged":true}}}}`)
	require.NoError(t, client.Write(ctx, initialize))
	result, err := client.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, initialize, <-requests)
	response := result.(*jsonrpc.Response)
	require.Equal(t, initialize.(*jsonrpc.Request).ID, response.ID)
	require.Contains(t, string(response.Result), `"name":"actual"`)
	for _, text := range []string{
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":42}}`,
		`{"jsonrpc":"2.0","id":"server-request","result":{"roots":[]}}`,
		`{"jsonrpc":"2.0","id":42,"method":"tools/list"}`,
	} {
		msg := wire(t, text)
		require.NoError(t, client.Write(ctx, msg))
		require.Equal(t, msg, <-requests)
	}
	result, err = client.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, wire(t, `{"jsonrpc":"2.0","id":42,"result":{"ok":true}}`), result)
	require.NoError(t, client.Close())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("bridge did not stop on EOF")
	}
}

func TestReadEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	a, b := gomcp.NewInMemoryTransports()
	local, err := a.Connect(ctx)
	require.NoError(t, err)
	defer local.Close()
	client, err := b.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()
	lastID := ""
	done := make(chan error, 1)
	go func() {
		done <- readEvents(ctx, strings.NewReader("id: evt-1\nevent: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":\"ask\",\"method\":\"roots/list\"}\n\n"), local, &lastID)
	}()
	msg, err := client.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "roots/list", msg.(*jsonrpc.Request).Method)
	require.Error(t, <-done)
	require.Equal(t, "evt-1", lastID)
}

func TestBridgeForwardsCancellationWhileRequestIsPending(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	started := make(chan struct{})
	cancelled := make(chan struct{})
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Method string `json:"method"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		if body.Method == "tools/call" {
			close(started)
			select {
			case <-cancelled:
			case <-r.Context().Done():
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		} else {
			require.Equal(t, "notifications/cancelled", body.Method)
			close(cancelled)
			w.WriteHeader(http.StatusAccepted)
		}
	}))
	defer gateway.Close()
	a, b := gomcp.NewInMemoryTransports()
	local, err := a.Connect(ctx)
	require.NoError(t, err)
	client, err := b.Connect(ctx)
	require.NoError(t, err)
	defer client.Close()
	done := make(chan error, 1)
	go func() { done <- bridge(ctx, local, gateway.URL, nil, http.DefaultTransport) }()
	require.NoError(t, client.Write(ctx, wire(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slow"}}`)))
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("request never started")
	}
	require.NoError(t, client.Write(ctx, wire(t, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`)))
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("cancellation blocked behind pending request")
	}
	_, err = client.Read(ctx)
	require.NoError(t, err)
	require.NoError(t, client.Close())
	require.NoError(t, <-done)
}
