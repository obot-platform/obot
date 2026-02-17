// Package mockwordpress provides a mock WordPress MCP server for evals.
package mockwordpress

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// Tool names matching typical WordPress MCP servers.
const (
	ToolValidateCredential = "validate_credential"
	ToolGetSiteSettings   = "get_site_settings"
	ToolCreatePost        = "create_post"
	ToolListPosts         = "list_posts"
)

// Server is a mock MCP server that exposes WordPress-like tools.
// When Config is set (FromEnv), validate_credential can optionally check the real WordPress REST API.
type Server struct {
	mu     sync.Mutex
	config Config
	server *http.Server
	ln     net.Listener
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
	JSONRPC string     `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError  `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
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

// NewServer creates a mock WordPress MCP server. Config is read from env in Start().
func NewServer(config Config) *Server {
	return &Server{config: config}
}

// Start starts the HTTP server on the given address (e.g. "127.0.0.1:0").
// Returns the base URL and any error.
func (s *Server) Start(addr string) (baseURL string, err error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	baseURL = "http://" + ln.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleMCP)
	s.mu.Lock()
	s.server = &http.Server{Handler: mux}
	s.ln = ln
	// Refresh config from env in case it was set after NewServer
	s.config = FromEnv()
	s.mu.Unlock()
	go func() {
		_ = s.server.Serve(ln)
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
	if s.server != nil {
		_ = s.server.Close()
		s.server = nil
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
		result = map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
			"serverInfo":      map[string]interface{}{"name": "eval-mock-wordpress", "version": "0.0.1"},
		}
		w.Header().Set("Mcp-Session-Id", "eval-mock-wp-session")
	case "notifications/initialized":
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":null}`))
		return
	case "tools/list":
		result = map[string]interface{}{
			"tools": s.toolDefs(),
		}
	case "tools/call":
		var params toolsCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeRPCError(w, id, -32602, "Invalid params")
			return
		}
		content, isErr := s.handleToolCall(params.Name, params.Arguments)
		result = &toolsCallResult{Content: []contentItem{{Type: "text", Text: content}}, IsError: isErr}
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

func (s *Server) toolDefs() []toolDef {
	return []toolDef{
		{
			Name:        ToolValidateCredential,
			Description: "Validate the connection credentials for the WordPress server.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"site_url":  map[string]interface{}{"type": "string", "description": "WordPress site URL"},
					"username":  map[string]interface{}{"type": "string", "description": "Username"},
					"password":  map[string]interface{}{"type": "string", "description": "Application password"},
				},
			},
		},
		{
			Name:        ToolGetSiteSettings,
			Description: "Retrieve the current WordPress site settings.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
		{
			Name:        ToolCreatePost,
			Description: "Create a new WordPress post.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"title":   map[string]interface{}{"type": "string"},
					"content": map[string]interface{}{"type": "string"},
					"status":  map[string]interface{}{"type": "string"},
				},
			},
		},
		{
			Name:        ToolListPosts,
			Description: "List recent WordPress posts.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
		},
	}
}

func (s *Server) handleToolCall(name string, args map[string]interface{}) (text string, isError bool) {
	s.mu.Lock()
	cfg := s.config
	s.mu.Unlock()

	switch name {
	case ToolValidateCredential:
		return s.validateCredential(cfg, args)
	case ToolGetSiteSettings:
		return s.getSiteSettings(cfg), false
	case ToolCreatePost:
		return s.createPost(cfg, args), false
	case ToolListPosts:
		return s.listPosts(cfg), false
	default:
		return `{"error":"Unknown tool: ` + name + `"}`, true
	}
}

func (s *Server) validateCredential(cfg Config, args map[string]interface{}) (string, bool) {
	// If tool passed credentials, use them; otherwise use config from env.
	url := str(args, "site_url", "url", "siteUrl")
	if url == "" {
		url = cfg.WebsiteURL
	}
	user := str(args, "username", "user")
	if user == "" {
		user = cfg.Username
	}
	pass := str(args, "password", "application_password", "app_password")
	if pass == "" {
		pass = cfg.AppPassword
	}

	if url == "" || user == "" || pass == "" {
		return `{"valid":false,"message":"missing site URL, username, or application password"}`, true
	}
	url = strings.TrimSuffix(url, "/")

	// Optional: validate against real WordPress REST API
	ok, msg := validateWordPressCredential(url, user, pass)
	if ok {
		return `{"valid":true,"message":"Credentials validated successfully"}`, false
	}
	return `{"valid":false,"message":"` + escapeJSON(msg) + `"}`, true
}

func (s *Server) getSiteSettings(cfg Config) string {
	name := "Eval Mock WordPress"
	if cfg.WebsiteURL != "" {
		name = cfg.WebsiteURL
	}
	return `{"name":"` + escapeJSON(name) + `","url":"` + escapeJSON(cfg.WebsiteURL) + `","description":"Mock WordPress for evals"}` 
}

func (s *Server) createPost(cfg Config, args map[string]interface{}) string {
	title := str(args, "title", "post_title")
	if title == "" {
		title = "Eval test post"
	}
	// Return a fake post URL; when config points to real WP, you could POST to wp/v2/posts and return real URL.
	base := cfg.WebsiteURL
	if base == "" {
		base = "https://testsite041stg.wpenginepowered.com"
	}
	fakeURL := strings.TrimSuffix(base, "/") + "/?p=1"
	return `{"id":1,"url":"` + fakeURL + `","title":"` + escapeJSON(title) + `","status":"draft"}`
}

func (s *Server) listPosts(cfg Config) string {
	return `{"posts":[],"total":0}`
}

func str(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func escapeJSON(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\\", "\\\\"), "\"", "\\\"")
}

// validateWordPressCredential checks the WordPress REST API with Basic auth.
func validateWordPressCredential(siteURL, username, appPassword string) (bool, string) {
	url := siteURL + "/wp-json/wp/v2/users/me"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false, err.Error()
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+appPassword)))
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 200 {
			body = body[:200]
		}
		return false, "HTTP " + strconv.Itoa(resp.StatusCode) + ": " + string(body)
	}
	return true, ""
}

func writeRPCError(w http.ResponseWriter, id interface{}, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	})
}
