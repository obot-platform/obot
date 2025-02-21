package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/obot-platform/obot/apiclient/types"
)

func (c *Client) UpdateWorkflow(ctx context.Context, id string, manifest types.WorkflowManifest) (*types.Workflow, error) {
	_, resp, err := c.putJSON(ctx, fmt.Sprintf("/workflows/%s", id), manifest)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.Workflow{})
}

func (c *Client) GetWorkflow(ctx context.Context, id string) (*types.Workflow, error) {
	_, resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/workflows/%s", id), nil)
	if err != nil {
		return nil, err
	}

	return toObject(resp, &types.Workflow{})
}

type ListWorkflowExecutionsOptions struct {
	ThreadID string
}

func (c *Client) ListWorkflowExecutions(ctx context.Context, workflowID string, opts ListWorkflowExecutionsOptions) (result types.WorkflowExecutionList, err error) {
	defer func() {
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].Metadata.Created.Time.Before(result.Items[j].Metadata.Created.Time)
		})
	}()

	url := fmt.Sprintf("/workflows/%s/executions", workflowID)
	if opts.ThreadID != "" {
		url = fmt.Sprintf("/threads/%s/workflows/%s/executions", opts.ThreadID, workflowID)
	}

	_, resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, err = toObject(resp, &result)
	return
}

type ListWorkflowsOptions struct {
	ThreadID string
}

func (c *Client) ListWorkflows(ctx context.Context, opts ListWorkflowsOptions) (result types.WorkflowList, err error) {
	defer func() {
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].Metadata.Created.Time.Before(result.Items[j].Metadata.Created.Time)
		})
	}()

	url := "/workflows"
	if opts.ThreadID != "" {
		url = fmt.Sprintf("/threads/%s/workflows", opts.ThreadID)
	}

	_, resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, err = toObject(resp, &result)
	if err != nil {
		return result, err
	}

	return
}

func (c *Client) DeleteWorkflow(ctx context.Context, id string) error {
	_, resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/workflows/%s", id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) AuthenticateWorkflow(ctx context.Context, wfID string) (*types.InvokeResponse, error) {
	url := fmt.Sprintf("/workflows/%s/authenticate", wfID)

	_, resp, err := c.doRequest(ctx, http.MethodPost, url, nil, "Accept", "text/event-stream")
	if err != nil {
		return nil, err
	}

	return &types.InvokeResponse{
		Events:   toStream[types.Progress](resp),
		ThreadID: resp.Header.Get("X-Obot-Thread-Id"),
	}, nil
}
