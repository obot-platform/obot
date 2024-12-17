package apiclient

import (
	"context"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
)

type ListFileOptions struct {
	AgentID    string
	WorkflowID string
	ThreadID   string
}

func (c *Client) ListFiles(ctx context.Context, opts ListFileOptions) (result types.FileList, err error) {
	path := "/files"
	if opts.AgentID != "" {
		path = "/agents/" + opts.AgentID + path
	} else if opts.WorkflowID != "" {
		path = "/workflows/" + opts.WorkflowID + path
	} else if opts.ThreadID != "" {
		path = "/threads/" + opts.ThreadID + path
	} else {
		return result, fmt.Errorf("missing agentID, workflowID, or threadID")
	}

	_, resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, err = toObject(resp, &result)
	return
}
