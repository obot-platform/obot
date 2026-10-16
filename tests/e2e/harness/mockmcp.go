//go:build e2e

package harness

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MockMCP is a remote MCP server for guardrail filter tests. Its tools, resources and
// prompts take base64 payloads for what they return, so a request never contains the
// secret a response carries:
//
//   - tool "echo" returns its "text" argument;
//   - tool "leak" returns the decoded "payload" argument;
//   - resource template e2e://doc/{payload} returns the decoded (base64url) payload;
//   - prompt "brief" returns a message with its "topic" argument and decoded "payload".
//
// It records the arguments it receives.
type MockMCP struct {
	URL string

	server   *http.Server
	listener net.Listener

	mu       sync.Mutex
	received []string
	// raw holds every HTTP request body the server received.
	raw []string
}

// StartMockMCP starts the MCP server on a random localhost port. Its endpoint is URL.
func StartMockMCP() (*MockMCP, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	m := &MockMCP{URL: "http://" + l.Addr().String() + "/mcp", listener: l}

	s := gomcp.NewServer(&gomcp.Implementation{Name: "obot-e2e-mock", Version: "1.0.0"}, nil)
	s.AddTool(&gomcp.Tool{
		Name:        "echo",
		Description: "Echoes its text argument.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
	}, func(_ context.Context, req *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		args := m.record(req.Params.Arguments)
		text, _ := args["text"].(string)
		return &gomcp.CallToolResult{Content: []gomcp.Content{&gomcp.TextContent{Text: "echo: " + text}}}, nil
	})
	s.AddTool(&gomcp.Tool{
		Name:        "leak",
		Description: "Returns the decoded base64 payload.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{"payload": map[string]any{"type": "string"}}},
	}, func(_ context.Context, req *gomcp.CallToolRequest) (*gomcp.CallToolResult, error) {
		args := m.record(req.Params.Arguments)
		payload, _ := args["payload"].(string)
		decoded, _ := base64.StdEncoding.DecodeString(payload)
		return &gomcp.CallToolResult{Content: []gomcp.Content{&gomcp.TextContent{Text: "leaked: " + string(decoded)}}}, nil
	})
	s.AddResourceTemplate(&gomcp.ResourceTemplate{
		Name:        "doc",
		URITemplate: "e2e://doc/{payload}",
		MIMEType:    "text/plain",
	}, func(_ context.Context, req *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
		payload := strings.TrimPrefix(req.Params.URI, "e2e://doc/")
		decoded, _ := base64.RawURLEncoding.DecodeString(payload)
		return &gomcp.ReadResourceResult{Contents: []*gomcp.ResourceContents{{
			URI: req.Params.URI, MIMEType: "text/plain", Text: "document: " + string(decoded),
		}}}, nil
	})
	s.AddPrompt(&gomcp.Prompt{
		Name:      "brief",
		Arguments: []*gomcp.PromptArgument{{Name: "topic"}, {Name: "payload"}},
	}, func(_ context.Context, req *gomcp.GetPromptRequest) (*gomcp.GetPromptResult, error) {
		data, _ := json.Marshal(req.Params.Arguments)
		m.mu.Lock()
		m.received = append(m.received, string(data))
		m.mu.Unlock()
		decoded, _ := base64.StdEncoding.DecodeString(req.Params.Arguments["payload"])
		return &gomcp.GetPromptResult{Messages: []*gomcp.PromptMessage{{
			Role:    "user",
			Content: &gomcp.TextContent{Text: "Brief about " + req.Params.Arguments["topic"] + ". " + string(decoded)},
		}}}, nil
	})

	mux := http.NewServeMux()
	handler := gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server { return s }, nil)
	mux.Handle("/mcp", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			m.mu.Lock()
			m.raw = append(m.raw, string(body))
			m.mu.Unlock()
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		handler.ServeHTTP(w, r)
	}))
	m.server = &http.Server{Handler: mux}
	go func() { _ = m.server.Serve(l) }()
	return m, nil
}

func (m *MockMCP) record(raw any) map[string]any {
	var args map[string]any
	switch v := raw.(type) {
	case json.RawMessage:
		_ = json.Unmarshal(v, &args)
	case map[string]any:
		args = v
	default:
		data, _ := json.Marshal(v)
		_ = json.Unmarshal(data, &args)
	}
	data, _ := json.Marshal(args)
	m.mu.Lock()
	m.received = append(m.received, string(data))
	m.mu.Unlock()
	return args
}

// Received returns the JSON-encoded arguments the server received that contain substr.
func (m *MockMCP) Received(substr string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, r := range m.received {
		if strings.Contains(r, substr) {
			out = append(out, r)
		}
	}
	return out
}

// RawRequests returns the HTTP request bodies the server received that contain substr.
func (m *MockMCP) RawRequests(substr string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, r := range m.raw {
		if strings.Contains(r, substr) {
			out = append(out, r)
		}
	}
	return out
}

// Close stops the server.
func (m *MockMCP) Close() {
	_ = m.server.Close()
}
