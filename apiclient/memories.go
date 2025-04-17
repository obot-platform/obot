package apiclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
)

// AddMemories adds memories to a project
func (c *Client) AddMemories(ctx context.Context, assistantID, projectID string, memories ...types.Memory) (*types.MemorySet, error) {
	url := fmt.Sprintf("/assistants/%s/projects/%s/memories", assistantID, projectID)

	_, resp, err := c.postJSON(ctx, url, memories)
	if err != nil {
		return nil, err
	}

	var result types.MemorySet
	_, err = toObject(resp, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// GetMemories retrieves all memories for a project
func (c *Client) GetMemories(ctx context.Context, assistantID, projectID string) (*types.MemorySet, error) {
	url := fmt.Sprintf("/assistants/%s/projects/%s/memories", assistantID, projectID)

	_, resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var result types.MemorySet
	_, err = toObject(resp, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
