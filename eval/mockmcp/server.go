// Package mockmcp provides a minimal MCP server for evals with deterministic tool output.
package mockmcp

import (
	"encoding/json"
	"net"
	"net/http"
	"sync"
)

const (
	// ToolEchoName is the single tool exposed by the mock server.
	ToolEchoName = "echo"
	// EchoParamMessage is the argument name for the echo tool.
	EchoParamMessage = "message"
)

// Server is a minimal MCP HTTP server that implements initialize, tools/list, and tools/call (echo tool).
type Server struct {
	mu       sync.Mutex
	listener *http.Server
	ln       net.Listener
}

// jsonRPCRequest is the generic JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is the generic JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError  `json:"error,omitempty"`
}

type rpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type initResult struct {
	ProtocolVersion string      `json:"protocolVersion"`
	Capabilities    initCaps    `json:"capabilities"`
	ServerInfo      serverInfo  `json:"serverInfo"`
}

type initCaps struct {
	Tools map[string]interface{} `json:"tools,omitempty"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolsListResult struct {
	Tools []toolDef `json:"tools"`
}

type toolDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]propSchema `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type propSchema struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type toolsCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type toolsCallResult struct {
	Content []contentItem `json:"content"`
	IsError bool          `json:"isError"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// echoTool is the single tool definition returned by tools/list.
var echoTool = toolDef{
	Name:        ToolEchoName,
	Description: "Echo back the given message (deterministic for evals)",
	InputSchema: inputSchema{
		Type: "object",
		Properties: map[string]propSchema{
			EchoParamMessage: {Type: "string", Description: "Message to echo back"},
		},
		Required: []string{EchoParamMessage},
	},
}

// NewServer creates a new mock MCP server. Call Start() to listen.
func NewServer() *Server {
	return &Server{}
}

// Start starts the HTTP server on the given address (e.g. "127.0.0.1:0" for random port).
// Returns the base URL (e.g. "http://127.0.0.1:12345") and any error.
func (s *Server) Start(addr string) (baseURL string, err error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	baseURL = "http://" + ln.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleMCP)
	s.mu.Lock()
	s.listener = &http.Server{Handler: mux}
	s.ln = ln
	s.mu.Unlock()
	go func() {
		_ = s.listener.Serve(ln)
	}()
	return baseURL, nil
}

// Close shuts down the server.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		_ = s.ln.Close()
		s.ln = nil
	}
	if s.listener != nil {
		_ = s.listener.Close()
		s.listener = nil
	}
	return nil
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req jsonRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPCError(w, nil, -32700, "Parse error")
		return
	}
	id := req.ID
	if id == nil {
		id = json.Number("0")
	}

	var result interface{}
	switch req.Method {
	case "initialize":
		result = &initResult{
			ProtocolVersion: "2024-11-05",
			Capabilities:    initCaps{Tools: map[string]interface{}{}},
			ServerInfo:      serverInfo{Name: "eval-mock-mcp", Version: "0.0.1"},
		}
		// Optional: set Mcp-Session-Id for clients that expect it
		w.Header().Set("Mcp-Session-Id", "eval-mock-session")
	case "notifications/initialized":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":null}`))
		return
	case "tools/list":
		result = &toolsListResult{Tools: []toolDef{echoTool}}
	case "tools/call":
		var params toolsCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeRPCError(w, id, -32602, "Invalid params")
			return
		}
		if params.Name != ToolEchoName {
			writeRPCError(w, id, -32602, "Unknown tool: "+params.Name)
			return
		}
		msg, _ := params.Arguments[EchoParamMessage].(string)
		result = &toolsCallResult{
			Content: []contentItem{{Type: "text", Text: msg}},
			IsError: false,
		}
	default:
		writeRPCError(w, id, -32601, "Method not found: "+req.Method)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func writeRPCError(w http.ResponseWriter, id interface{}, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	})
}
