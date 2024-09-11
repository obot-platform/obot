package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/otto/pkg/api"
	"github.com/gptscript-ai/otto/pkg/api/types"
)

type Client struct {
	BaseURL string
	Token   string
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader, headerKV ...string) (*http.Request, *http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if len(headerKV)%2 != 0 {
		return nil, nil, fmt.Errorf("length of headerKV must be even")
	}
	for i := 0; i < len(headerKV); i += 2 {
		req.Header.Add(headerKV[i], headerKV[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode > 399 {
		data, _ := io.ReadAll(resp.Body)
		msg := string(data)
		if len(msg) == 0 {
			msg = resp.Status
		}
		return nil, nil, &api.ErrHTTP{
			Code:    resp.StatusCode,
			Message: msg,
		}
	}
	return req, resp, err
}

type CreateOptions struct {
	Replace          bool
	ReplacedCallback func()
}

func (c *Client) UpdateAgent(ctx context.Context, id string, manifest []byte) (*types.Agent, error) {
	_, resp, err := c.doRequest(ctx, http.MethodPut, fmt.Sprintf("/agents/"+id), bytes.NewBuffer(manifest))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var agent types.Agent
	if err := json.NewDecoder(resp.Body).Decode(&agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (c *Client) DeleteAgent(ctx context.Context, id string) error {
	_, resp, err := c.doRequest(ctx, http.MethodDelete, fmt.Sprintf("/agents/"+id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

type InvokeOptions struct {
	ThreadID string
}

func (c *Client) Invoke(ctx context.Context, agentID string, input string, opt ...InvokeOptions) (*types.InvokeResponse, error) {
	var (
		opts InvokeOptions
	)
	for _, o := range opt {
		if o.ThreadID != "" {
			opts.ThreadID = o.ThreadID
		}
	}

	url := fmt.Sprintf("/invoke/%s?events=true", agentID)
	if opts.ThreadID != "" {
		url = fmt.Sprintf("/invoke/%s/threads/%s?events=true", agentID, opts.ThreadID)
	}

	_, resp, err := c.doRequest(ctx, http.MethodPost, url, bytes.NewBuffer([]byte(input)), "Accept", "text/event-stream")
	if err != nil {
		return nil, err
	}

	events := make(chan types.Progress)
	go func() {
		defer resp.Body.Close()
		defer close(events)
		lines := bufio.NewScanner(resp.Body)
		for lines.Scan() {
			var event types.Progress
			data := strings.TrimPrefix(lines.Text(), "data: ")
			if len(data) == 0 {
				continue
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				events <- types.Progress{
					Error: err.Error(),
				}
			} else {
				events <- event
			}
		}

		if err := lines.Err(); err != nil {
			events <- types.Progress{
				Error: err.Error(),
			}
		}
	}()

	return &types.InvokeResponse{
		Events:   events,
		RunID:    resp.Header.Get("X-Otto-Run-Id"),
		ThreadID: resp.Header.Get("X-Otto-Thread-Id"),
	}, nil
}

func (c *Client) CreateAgent(ctx context.Context, manifest []byte, opts ...CreateOptions) (*types.Agent, error) {
	var (
		opt CreateOptions
		cbs []func()
	)
	for _, o := range opts {
		opt.Replace = opt.Replace || o.Replace
		if o.ReplacedCallback != nil {
			cbs = append(cbs, o.ReplacedCallback)
		}
	}

	_, resp, err := c.doRequest(ctx, http.MethodPost, fmt.Sprintf("/agents?replace=%v", opt.Replace), bytes.NewBuffer(manifest))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.Header.Get("X-Otto-Replaced") == "true" {
		for _, cb := range cbs {
			cb()
		}
	}

	var agent types.Agent
	if err := json.NewDecoder(resp.Body).Decode(&agent); err != nil {
		return nil, err
	}
	return &agent, nil
}

func (c *Client) ListAgents(ctx context.Context) (result types.AgentList, err error) {
	defer func() {
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].Metadata.Created.Before(result.Items[j].Metadata.Created)
		})
	}()

	_, resp, err := c.doRequest(ctx, http.MethodGet, "/agents", nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&result)
	return
}

type ListThreadsOptions struct {
	AgentID string
}

func (c *Client) ListThreads(ctx context.Context, opts ...ListThreadsOptions) (result types.ThreadList, err error) {
	defer func() {
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].Created.Before(result.Items[j].Created)
		})
	}()

	var opt ListThreadsOptions
	for _, o := range opts {
		if o.AgentID != "" {
			opt.AgentID = o.AgentID
		}
	}
	url := "/threads"
	if opt.AgentID != "" {
		url = fmt.Sprintf("/agents/%s", opt.AgentID) + url
	}
	_, resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&result)
	return
}

type ListRunsOptions struct {
	AgentID  string
	ThreadID string
}

func (c *Client) DebugRun(ctx context.Context, runID string) (result types.RunDebug, err error) {
	_, resp, err := c.doRequest(ctx, http.MethodGet, fmt.Sprintf("/runs/%s/debug", runID), nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	result.Frames = map[string]gptscript.CallFrame{}
	err = json.NewDecoder(resp.Body).Decode(&result.Frames)
	return
}

func (c *Client) ListRuns(ctx context.Context, opts ...ListRunsOptions) (result types.RunList, err error) {
	defer func() {
		sort.Slice(result.Items, func(i, j int) bool {
			return result.Items[i].Created.Before(result.Items[j].Created)
		})
	}()
	var opt ListRunsOptions
	for _, o := range opts {
		if o.ThreadID != "" {
			opt.ThreadID = o.ThreadID
		}
		if o.AgentID != "" {
			opt.AgentID = o.AgentID
		}
	}
	url := "/runs"
	if opt.AgentID != "" && opt.ThreadID != "" {
		url = fmt.Sprintf("/agents/%s/threads/%s/runs", opt.AgentID, opt.ThreadID)
	} else if opt.AgentID != "" {
		url = fmt.Sprintf("/agents/%s/runs", opt.AgentID)
	} else if opt.ThreadID != "" {
		url = fmt.Sprintf("/threads/%s/runs", opt.ThreadID)
	}
	_, resp, err := c.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	err = json.NewDecoder(resp.Body).Decode(&result)
	return
}
