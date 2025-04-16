package apiclient

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sort"

	"github.com/obot-platform/obot/apiclient/types"
)

func (c *Client) DeleteThread(ctx context.Context, id string) error {
	_, resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/threads/%s", id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

type ThreadEventsOptions struct {
	Follow  bool
	RunID   string
	MaxRuns int
}

func (c *Client) ThreadEvents(ctx context.Context, threadID string, opts ThreadEventsOptions) (result <-chan types.Progress, err error) {
	path := fmt.Sprintf("/threads/%s/events?runID=%s&follow=%v", threadID, opts.RunID, opts.Follow)
	if opts.MaxRuns > 0 {
		path += fmt.Sprintf("&maxRuns=%d", opts.MaxRuns)
	}

	_, resp, err := c.doStream(ctx, http.MethodGet, path, nil)
	if err != nil {
		return
	}

	return toStream[types.Progress](resp), nil
}

type ListThreadsOptions struct {
	AgentID string
}

func (c *Client) UpdateThread(ctx context.Context, id string, thread types.ThreadManifest) (*types.Thread, error) {
	_, resp, err := c.putJSON(ctx, fmt.Sprintf("/threads/%s", id), thread)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return toObject(resp, &types.Thread{})
}

func (c *Client) GetThread(ctx context.Context, threadID string) (result *types.Thread, err error) {
	_, resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/threads/%s", threadID), nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	return toObject(resp, &types.Thread{})
}

func (c *Client) ListThreads(ctx context.Context, opts ListThreadsOptions) (result types.ThreadList, err error) {
	defer func() {
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].Created.Time.Before(result.Items[j].Created.Time)
		})
	}()

	url := "/threads"
	if opts.AgentID != "" {
		url = fmt.Sprintf("/agents/%s", opts.AgentID) + url
	}
	_, resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	_, err = toObject(resp, &result)
	return
}

func (c *Client) UploadThreadFile(ctx context.Context, projectID string, threadID string, assistantID string, filename string, fileContent []byte) error {
	_, resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/assistants/%s/projects/%s/threads/%s/files/%s", assistantID, projectID, threadID, filename), bytes.NewReader(fileContent))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (c *Client) UploadKnowledgeFile(ctx context.Context, projectID string, threadID string, assistantID string, filename string, fileContent []byte) error {
	_, resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/assistants/%s/projects/%s/threads/%s/knowledge-files/%s", assistantID, projectID, threadID, filename), bytes.NewReader(fileContent))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
