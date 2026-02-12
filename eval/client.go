package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultHTTPTimeout = 30 * time.Second

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

// ProjectV2 as returned by API.
type ProjectV2 struct {
	Metadata   map[string]interface{} `json:"metadata"`
	UserID    string                 `json:"userID,omitempty"`
	DisplayName string               `json:"displayName,omitempty"`
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

// NanobotAgent as returned by API.
type NanobotAgent struct {
	Metadata   map[string]interface{} `json:"metadata"`
	UserID    string                 `json:"userID,omitempty"`
	ProjectV2ID string                `json:"projectV2ID,omitempty"`
	ConnectURL string                 `json:"connectURL,omitempty"`
	DisplayName string               `json:"displayName,omitempty"`
	Description string               `json:"description,omitempty"`
	DefaultAgent string               `json:"defaultAgent,omitempty"`
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

// AgentID extracts ID from Metadata.
func AgentID(a *NanobotAgent) string {
	if a == nil || a.Metadata == nil {
		return ""
	}
	if id, ok := a.Metadata["id"].(string); ok {
		return id
	}
	if id, ok := a.Metadata["name"].(string); ok {
		return id
	}
	return ""
}
