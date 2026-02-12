package mockmcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a minimal MCP JSON-RPC client for evals.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient creates a client for the given MCP server base URL.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Call sends a JSON-RPC request and returns the result (or error).
func (c *Client) Call(method string, params interface{}) (result interface{}, err error) {
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

// CallEcho invokes the echo tool and returns the echoed text.
func (c *Client) CallEcho(message string) (string, error) {
	res, err := c.Call("tools/call", map[string]interface{}{
		"name":      ToolEchoName,
		"arguments": map[string]interface{}{EchoParamMessage: message},
	})
	if err != nil {
		return "", err
	}
	// result: { "content": [ { "type": "text", "text": "..." } ], "isError": false }
	m, ok := res.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected result type %T", res)
	}
	content, _ := m["content"].([]interface{})
	if len(content) == 0 {
		return "", fmt.Errorf("empty content")
	}
	first, _ := content[0].(map[string]interface{})
	text, _ := first["text"].(string)
	return text, nil
}
