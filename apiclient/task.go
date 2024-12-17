package apiclient

import (
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/obot-platform/obot/apiclient/types"
)

type ListTasksOptions struct {
	ThreadID    string
	AssistantID string
}

func (c *Client) ListTasks(ctx context.Context, opts ListTasksOptions) (result types.TaskList, err error) {
	defer func() {
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].Metadata.Created.Time.Before(result.Items[j].Metadata.Created.Time)
		})
	}()

	if opts.ThreadID == "" && opts.AssistantID == "" {
		return result, fmt.Errorf("either threadID or assistantID must be provided")
	}
	var url string
	if opts.ThreadID != "" {
		url = fmt.Sprintf("/threads/%s/tasks", opts.ThreadID)
	} else {
		url = fmt.Sprintf("/assistants/%s/tasks", opts.AssistantID)
	}
	_, resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()

	_, err = toObject(resp, &result)
	return
}

type UpdateTaskOptions struct {
	ThreadID    string
	AssistantID string
}

func (c *Client) UpdateTask(ctx context.Context, id string, manifest types.TaskManifest, opt UpdateTaskOptions) (*types.Task, error) {
	var url string
	if opt.ThreadID != "" {
		url = fmt.Sprintf("/threads/%s/tasks/%s", opt.ThreadID, id)
	} else {
		url = fmt.Sprintf("/assistants/%s/tasks/%s", opt.AssistantID, id)
	}

	_, resp, err := c.putJSON(ctx, url, manifest)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.Task{})
}

type CreateTaskOptions struct {
	ThreadID    string
	AssistantID string
}

func (c *Client) CreateTask(ctx context.Context, manifest types.TaskManifest, opt CreateTaskOptions) (*types.Task, error) {
	var url string
	if opt.ThreadID != "" {
		url = fmt.Sprintf("/threads/%s/tasks", opt.ThreadID)
	} else {
		url = fmt.Sprintf("/assistants/%s/tasks", opt.AssistantID)
	}

	_, resp, err := c.postJSON(ctx, url, manifest)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.Task{})
}

type ListTaskRunsOptions struct {
	ThreadID    string
	AssistantID string
}

func (c *Client) ListTaskRuns(ctx context.Context, taskID string, opts ListTaskRunsOptions) (result types.TaskRunList, err error) {
	defer func() {
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].Metadata.Created.Time.Before(result.Items[j].Metadata.Created.Time)
		})
	}()

	if opts.ThreadID == "" && opts.AssistantID == "" {
		return result, fmt.Errorf("either threadID or assistantID must be provided")
	}
	var url string
	if opts.ThreadID != "" {
		url = fmt.Sprintf("/threads/%s/tasks/%s/runs", opts.ThreadID, taskID)
	} else {
		url = fmt.Sprintf("/assistants/%s/tasks/%s/runs", opts.AssistantID, taskID)
	}

	_, resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, err = toObject(resp, &result)
	return
}

type TaskRunOptions struct {
	ThreadID    string
	AssistantID string
}

func (c *Client) RunTask(ctx context.Context, taskID string, input string, opts TaskRunOptions) (*types.TaskRun, error) {
	var url string
	if opts.ThreadID != "" {
		url = fmt.Sprintf("/threads/%s/tasks/%s/runs", opts.ThreadID, taskID)
	} else {
		url = fmt.Sprintf("/assistants/%s/tasks/%s/runs", opts.AssistantID, taskID)
	}

	_, resp, err := c.postJSON(ctx, url, input)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.TaskRun{})
}
