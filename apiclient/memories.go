package apiclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
)

// CreateMemory adds a single memory to a project
func (c *Client) CreateMemory(ctx context.Context, assistantID, projectID string, content string) (*types.Memory, error) {
	url := fmt.Sprintf("/assistants/%s/projects/%s/memories", assistantID, projectID)
	_, resp, err := c.postJSON(ctx, url, types.Memory{
		Content: content,
	})
	if err != nil {
		return nil, err
	}

	return decodeResponse[types.Memory](resp)
}

// ListMemories retrieves all memories for a project
func (c *Client) ListMemories(ctx context.Context, assistantID, projectID string) (*types.MemoryList, error) {
	url := fmt.Sprintf("/assistants/%s/projects/%s/memories", assistantID, projectID)

	_, resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	return decodeResponse[types.MemoryList](resp)
}

// UpdateMemory updates an existing memory by its index
func (c *Client) UpdateMemory(ctx context.Context, assistantID, projectID, memoryID string, content string) (*types.Memory, error) {
	url := fmt.Sprintf("/assistants/%s/projects/%s/memories/%s", assistantID, projectID, memoryID)
	_, resp, err := c.putJSON(ctx, url, types.Memory{
		Content: content,
	})
	if err != nil {
		return nil, err
	}

	return decodeResponse[types.Memory](resp)
}

// DeleteMemory deletes a memory by its index
func (c *Client) DeleteMemory(ctx context.Context, assistantID, projectID, memoryID string) (*types.MemoryList, error) {
	url := fmt.Sprintf("/assistants/%s/projects/%s/memories/%s", assistantID, projectID, memoryID)

	_, resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

	return decodeResponse[types.MemoryList](resp)
}

// DeleteMemories deletes all memories for a project
func (c *Client) DeleteMemories(ctx context.Context, assistantID, projectID string) (*types.MemoryList, error) {
	url := fmt.Sprintf("/assistants/%s/projects/%s/memories", assistantID, projectID)

	_, resp, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

	return decodeResponse[types.MemoryList](resp)
}
