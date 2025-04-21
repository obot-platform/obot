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

	var result types.Memory
	_, err = toObject(resp, &result)
	return &result, err
}

// ListMemories retrieves all memories for a project
func (c *Client) ListMemories(ctx context.Context, assistantID, projectID string) (*types.MemoryList, error) {
	url := fmt.Sprintf("/assistants/%s/projects/%s/memories", assistantID, projectID)

	_, resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result types.MemoryList
	_, err = toObject(resp, &result)
	return &result, err
}

// UpdateMemory updates an existing memory by ID
func (c *Client) UpdateMemory(ctx context.Context, assistantID, projectID, memoryID string, content string) (*types.Memory, error) {
	url := fmt.Sprintf("/assistants/%s/projects/%s/memories/%s", assistantID, projectID, memoryID)
	_, resp, err := c.putJSON(ctx, url, types.Memory{
		Content: content,
	})
	if err != nil {
		return nil, err
	}

	var result types.Memory
	_, err = toObject(resp, &result)
	return &result, err
}

// DeleteMemory deletes a memory by ID
func (c *Client) DeleteMemory(ctx context.Context, assistantID, projectID, memoryID string) error {
	url := fmt.Sprintf("/assistants/%s/projects/%s/memories/%s", assistantID, projectID, memoryID)

	_, _, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	return err
}

// DeleteMemories deletes all memories for a project
func (c *Client) DeleteMemories(ctx context.Context, assistantID, projectID string) error {
	url := fmt.Sprintf("/assistants/%s/projects/%s/memories", assistantID, projectID)

	_, _, err := c.doRequest(ctx, http.MethodDelete, url, nil)
	return err
}
