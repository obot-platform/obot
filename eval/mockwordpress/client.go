package mockwordpress

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal MCP JSON-RPC client for the mock WordPress server.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a client for the given MCP server base URL.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Call sends a JSON-RPC request and returns the result (or error).
func (c *Client) Call(method string, params interface{}) (interface{}, error) {
	body := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  params,
	}
	enc, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Post(c.baseURL+"/", "application/json", bytes.NewReader(enc))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Result interface{} `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Error != nil {
		return nil, fmt.Errorf("rpc error %d: %s", out.Error.Code, out.Error.Message)
	}
	return out.Result, nil
}

// ValidateCredential invokes validate_credential. If args is nil, the server uses config from env.
func (c *Client) ValidateCredential(args map[string]interface{}) (valid bool, message string, err error) {
	if args == nil {
		args = map[string]interface{}{}
	}
	res, err := c.Call("tools/call", map[string]interface{}{
		"name":      ToolValidateCredential,
		"arguments": args,
	})
	if err != nil {
		return false, "", err
	}
	m, ok := res.(map[string]interface{})
	if !ok {
		return false, "", fmt.Errorf("unexpected result type %T", res)
	}
	content, _ := m["content"].([]interface{})
	if len(content) == 0 {
		return false, "", fmt.Errorf("empty content")
	}
	first, _ := content[0].(map[string]interface{})
	text, _ := first["text"].(string)
	isErr, _ := m["isError"].(bool)
	// Simple parse: look for "valid":true in text
	if isErr {
		return false, text, nil
	}
	return strings.Contains(text, `"valid":true`), text, nil
}

// CreatePost invokes create_post and returns the response text.
func (c *Client) CreatePost(title, content, status string) (string, error) {
	args := map[string]interface{}{}
	if title != "" {
		args["title"] = title
	}
	if content != "" {
		args["content"] = content
	}
	if status != "" {
		args["status"] = status
	}
	res, err := c.Call("tools/call", map[string]interface{}{
		"name":      ToolCreatePost,
		"arguments": args,
	})
	if err != nil {
		return "", err
	}
	m, ok := res.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected result type %T", res)
	}
	contentList, _ := m["content"].([]interface{})
	if len(contentList) == 0 {
		return "", fmt.Errorf("empty content")
	}
	first, _ := contentList[0].(map[string]interface{})
	text, _ := first["text"].(string)
	return text, nil
}
