package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

const defaultHTTPTimeout = 30 * time.Second

// extractIDFromJSON sets id and name from JSON body (top-level or under "metadata").
func extractIDFromJSON(body []byte, id, name *string) {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return
	}
	if v, _ := m["id"].(string); v != "" {
		*id = v
	}
	if v, _ := m["name"].(string); v != "" {
		*name = v
	}
	if *id != "" && *name != "" {
		return
	}
	if meta, _ := m["metadata"].(map[string]interface{}); meta != nil {
		if v, _ := meta["id"].(string); v != "" && *id == "" {
			*id = v
		}
		if v, _ := meta["name"].(string); v != "" && *name == "" {
			*name = v
		}
	}
}

// Client is an HTTP client for Obot nanobot-related APIs (projectsv2, agents, launch).
type Client struct {
	baseURL    string
	authHeader string
	http       *http.Client
}

// NewClient creates a client. authHeader can be "Bearer <token>" or "Cookie: name=value" etc.
func NewClient(baseURL, authHeader string) (*Client, error) {
	baseURL = strings.TrimSuffix(baseURL, "/")
	return &Client{
		baseURL:    baseURL,
		authHeader: authHeader,
		http:       &http.Client{Timeout: defaultHTTPTimeout},
	}, nil
}

// do sends a request and returns response body and status. Caller checks status.
func (c *Client) do(method, path string, body any) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.authHeader != "" {
		// Allow "Authorization: Bearer x" or "Cookie: name=value"
		trimmed := strings.TrimSpace(c.authHeader)
		if len(trimmed) > 7 && strings.EqualFold(trimmed[:7], "cookie:") {
			req.Header.Set("Cookie", strings.TrimSpace(trimmed[7:]))
		} else {
			req.Header.Set("Authorization", c.authHeader)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return out, resp.StatusCode, nil
}

// doURL sends a request to an absolute URL (e.g. connectURL) with same auth and optional mcp-session-id.
func (c *Client) doURL(method, rawURL string, body any, sessionID string) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.authHeader != "" {
		trimmed := strings.TrimSpace(c.authHeader)
		if len(trimmed) > 7 && strings.EqualFold(trimmed[:7], "cookie:") {
			req.Header.Set("Cookie", strings.TrimSpace(trimmed[7:]))
		} else {
			req.Header.Set("Authorization", trimmed)
		}
	}
	if sessionID != "" {
		req.Header.Set("mcp-session-id", sessionID)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return out, resp.StatusCode, nil
}

// MCPInitialize sends initialize to connectURL and returns session ID from response header.
func (c *Client) MCPInitialize(connectURL string) (sessionID string, status int, err error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.New().String(),
		"method":  "initialize",
		"params": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]interface{}{},
			"clientInfo":      map[string]interface{}{"name": "eval", "version": "0.0.1"},
		},
	}
	req, _ := http.NewRequest("POST", connectURL, bytes.NewReader(mustMarshal(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.authHeader != "" {
		trimmed := strings.TrimSpace(c.authHeader)
		if len(trimmed) > 7 && strings.EqualFold(trimmed[:7], "cookie:") {
			req.Header.Set("Cookie", strings.TrimSpace(trimmed[7:]))
		} else {
			req.Header.Set("Authorization", trimmed)
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	sessionID = resp.Header.Get("mcp-session-id")
	if sessionID == "" {
		sessionID = resp.Header.Get("Mcp-Session-Id")
	}
	return sessionID, resp.StatusCode, nil
}

func mustMarshal(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

// MCPInitialized sends the initialized notification.
func (c *Client) MCPInitialized(connectURL, sessionID string) (int, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "initialized",
		"params":  map[string]interface{}{},
	}
	_, status, err := c.doURL("POST", connectURL, payload, sessionID)
	return status, err
}

// MCPToolsCall sends tools/call to connectURL and returns response body and status.
func (c *Client) MCPToolsCall(connectURL, sessionID, toolName string, arguments map[string]interface{}) ([]byte, int, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.New().String(),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": arguments,
		},
	}
	return c.doURL("POST", connectURL, payload, sessionID)
}

// MCPChatWithNanobot sends chat-with-nanobot tool call and returns response body and status.
func (c *Client) MCPChatWithNanobot(connectURL, sessionID, prompt string) ([]byte, int, error) {
	return c.MCPToolsCall(connectURL, sessionID, "chat-with-nanobot", map[string]interface{}{
		"prompt":      prompt,
		"attachments": []interface{}{},
	})
}

// MCPChatWithNanobotWithTimeout sends chat-with-nanobot with a custom HTTP timeout (e.g. for long-running workflows).
func (c *Client) MCPChatWithNanobotWithTimeout(connectURL, sessionID, prompt string, timeout time.Duration) ([]byte, int, error) {
	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      uuid.New().String(),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "chat-with-nanobot",
			"arguments": map[string]interface{}{
				"prompt":      prompt,
				"attachments": []interface{}{},
			},
		},
	}
	return c.doURLWithTimeout("POST", connectURL, payload, sessionID, timeout)
}

// doURLWithTimeout is like doURL but uses a custom timeout for this request.
func (c *Client) doURLWithTimeout(method, rawURL string, body any, sessionID string, timeout time.Duration) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, rawURL, bodyReader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if c.authHeader != "" {
		trimmed := strings.TrimSpace(c.authHeader)
		if len(trimmed) > 7 && strings.EqualFold(trimmed[:7], "cookie:") {
			req.Header.Set("Cookie", strings.TrimSpace(trimmed[7:]))
		} else {
			req.Header.Set("Authorization", trimmed)
		}
	}
	if sessionID != "" {
		req.Header.Set("mcp-session-id", sessionID)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return out, resp.StatusCode, nil
}

// CreateMCPServerFromCatalog creates an MCP server from a catalog entry (e.g. WordPress).
// Body: {"catalogEntryID": "<entry-id>", "manifest": {}}. Returns server ID from response.
func (c *Client) CreateMCPServerFromCatalog(catalogEntryID string) (serverID string, status int, err error) {
	body, status, err := c.do("POST", "/api/mcp-servers", map[string]interface{}{
		"catalogEntryID": catalogEntryID,
		"manifest":       map[string]interface{}{},
	})
	if err != nil {
		return "", status, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return "", status, fmt.Errorf("create MCP server: %s", string(body))
	}
	var id, name string
	extractIDFromJSON(body, &id, &name)
	if id != "" {
		return id, status, nil
	}
	if name != "" {
		return name, status, nil
	}
	// Fallback: parse metadata
	var m struct {
		Metadata struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"metadata"`
	}
	if json.Unmarshal(body, &m) == nil && (m.Metadata.ID != "" || m.Metadata.Name != "") {
		if m.Metadata.ID != "" {
			return m.Metadata.ID, status, nil
		}
		return m.Metadata.Name, status, nil
	}
	return "", status, fmt.Errorf("create MCP server: response missing server id")
}

// ConfigureMCPServer sets credentials/env for an MCP server (e.g. WORDPRESS_SITE, WORDPRESS_USERNAME, WORDPRESS_PASSWORD).
func (c *Client) ConfigureMCPServer(serverID string, envVars map[string]string) (int, error) {
	_, status, err := c.do("POST", "/api/mcp-servers/"+serverID+"/configure", envVars)
	return status, err
}

// LaunchMCPServer launches an MCP server deployment.
func (c *Client) LaunchMCPServer(serverID string) (int, error) {
	_, status, err := c.do("POST", "/api/mcp-servers/"+serverID+"/launch", map[string]interface{}{})
	return status, err
}

// VersionResponse is the shape of GET /api/version (we only need nanobotIntegration).
type VersionResponse struct {
	NanobotIntegration bool `json:"nanobotIntegration"`
}

// GetVersion returns version info. Used to check if nanobot is enabled.
func (c *Client) GetVersion() (*VersionResponse, int, error) {
	body, status, err := c.do("GET", "/api/version", nil)
	if err != nil {
		return nil, 0, err
	}
	if status != http.StatusOK {
		return nil, status, nil
	}
	var v VersionResponse
	if err := json.Unmarshal(body, &v); err != nil {
		return nil, status, err
	}
	return &v, status, nil
}

// ProjectV2 as returned by API (id/name may be top-level from embedded metadata).
type ProjectV2 struct {
	ID          string                 `json:"id,omitempty"`
	Name        string                 `json:"name,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
	UserID      string                 `json:"userID,omitempty"`
	DisplayName string                 `json:"displayName,omitempty"`
}

// CreateProjectV2 creates a project. Returns created project or error.
func (c *Client) CreateProjectV2(displayName string) (*ProjectV2, int, error) {
	body, status, err := c.do("POST", "/api/projectsv2", map[string]string{"displayName": displayName})
	if err != nil {
		return nil, 0, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return nil, status, fmt.Errorf("create project: %s", string(body))
	}
	var p ProjectV2
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, status, err
	}
	// API may return id/name at top level (embedded metadata); ensure we capture them
	if ProjectID(&p) == "" {
		extractIDFromJSON(body, &p.ID, &p.Name)
		if p.Metadata == nil {
			p.Metadata = make(map[string]interface{})
		}
		if p.ID != "" {
			p.Metadata["id"] = p.ID
		} else if p.Name != "" {
			p.Metadata["name"] = p.Name
		}
	}
	return &p, status, nil
}

// ListProjectsV2 returns projects (items array).
func (c *Client) ListProjectsV2() ([]ProjectV2, int, error) {
	body, status, err := c.do("GET", "/api/projectsv2", nil)
	if err != nil {
		return nil, 0, err
	}
	if status != http.StatusOK {
		return nil, status, fmt.Errorf("list projects: %s", string(body))
	}
	var out struct {
		Items []ProjectV2 `json:"items"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, status, err
	}
	return out.Items, status, nil
}

// GetProjectV2 returns one project by ID.
func (c *Client) GetProjectV2(projectID string) (*ProjectV2, int, error) {
	body, status, err := c.do("GET", "/api/projectsv2/"+projectID, nil)
	if err != nil {
		return nil, 0, err
	}
	if status != http.StatusOK {
		return nil, status, nil
	}
	var p ProjectV2
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, status, err
	}
	return &p, status, nil
}

// DeleteProjectV2 deletes a project.
func (c *Client) DeleteProjectV2(projectID string) (int, error) {
	_, status, err := c.do("DELETE", "/api/projectsv2/"+projectID, nil)
	return status, err
}

// NanobotAgent as returned by API (id/name may be top-level from embedded metadata).
type NanobotAgent struct {
	ID           string                 `json:"id,omitempty"`
	Name         string                 `json:"name,omitempty"`
	Metadata     map[string]interface{} `json:"metadata"`
	UserID       string                 `json:"userID,omitempty"`
	ProjectV2ID  string                 `json:"projectV2ID,omitempty"`
	ConnectURL   string                 `json:"connectURL,omitempty"`
	DisplayName  string                 `json:"displayName,omitempty"`
	Description  string                 `json:"description,omitempty"`
	DefaultAgent string                 `json:"defaultAgent,omitempty"`
}

// CreateAgent creates a nanobot agent in a project.
func (c *Client) CreateAgent(projectID, displayName, description string) (*NanobotAgent, int, error) {
	payload := map[string]string{"displayName": displayName}
	if description != "" {
		payload["description"] = description
	}
	body, status, err := c.do("POST", "/api/projectsv2/"+projectID+"/agents", payload)
	if err != nil {
		return nil, 0, err
	}
	if status != http.StatusOK && status != http.StatusCreated {
		return nil, status, fmt.Errorf("create agent: %s", string(body))
	}
	var a NanobotAgent
	if err := json.Unmarshal(body, &a); err != nil {
		return nil, status, err
	}
	return &a, status, nil
}

// ListAgents returns agents for a project.
func (c *Client) ListAgents(projectID string) ([]NanobotAgent, int, error) {
	body, status, err := c.do("GET", "/api/projectsv2/"+projectID+"/agents", nil)
	if err != nil {
		return nil, 0, err
	}
	if status != http.StatusOK {
		return nil, status, fmt.Errorf("list agents: %s", string(body))
	}
	var out struct {
		Items []NanobotAgent `json:"items"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, status, err
	}
	return out.Items, status, nil
}

// GetAgent returns one agent by ID.
func (c *Client) GetAgent(projectID, agentID string) (*NanobotAgent, int, error) {
	body, status, err := c.do("GET", "/api/projectsv2/"+projectID+"/agents/"+agentID, nil)
	if err != nil {
		return nil, 0, err
	}
	if status != http.StatusOK {
		return nil, status, nil
	}
	var a NanobotAgent
	if err := json.Unmarshal(body, &a); err != nil {
		return nil, status, err
	}
	return &a, status, nil
}

// UpdateAgent updates an agent manifest.
func (c *Client) UpdateAgent(projectID, agentID string, displayName, description string) (int, error) {
	payload := map[string]string{"displayName": displayName, "description": description}
	_, status, err := c.do("PUT", "/api/projectsv2/"+projectID+"/agents/"+agentID, payload)
	return status, err
}

// LaunchAgent launches the agent (ensures MCP server is deployed and ready).
func (c *Client) LaunchAgent(projectID, agentID string) (int, error) {
	_, status, err := c.do("POST", "/api/projectsv2/"+projectID+"/agents/"+agentID+"/launch", nil)
	return status, err
}

// DeleteAgent deletes an agent.
func (c *Client) DeleteAgent(projectID, agentID string) (int, error) {
	_, status, err := c.do("DELETE", "/api/projectsv2/"+projectID+"/agents/"+agentID, nil)
	return status, err
}

// ProjectID extracts ID from Metadata (API uses id from MetadataFrom).
func ProjectID(p *ProjectV2) string {
	if p == nil || p.Metadata == nil {
		return ""
	}
	if id, ok := p.Metadata["id"].(string); ok {
		return id
	}
	if id, ok := p.Metadata["name"].(string); ok {
		return id
	}
	return ""
}

// AgentID extracts ID from top-level or Metadata (API may return id/name at top level).
func AgentID(a *NanobotAgent) string {
	if a == nil {
		return ""
	}
	if a.ID != "" {
		return a.ID
	}
	if a.Name != "" {
		return a.Name
	}
	if a.Metadata != nil {
		if id, ok := a.Metadata["id"].(string); ok {
			return id
		}
		if id, ok := a.Metadata["name"].(string); ok {
			return id
		}
	}
	return ""
}
