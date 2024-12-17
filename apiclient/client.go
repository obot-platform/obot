package apiclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
)

var log = logger.Package()

type Client struct {
	BaseURL      string
	Token        string
	Cookie       *http.Cookie
	tokenFetcher func(context.Context, string) (string, error)
}

func (c *Client) WithTokenFetcher(f func(context.Context, string) (string, error)) *Client {
	n := *c
	n.tokenFetcher = f
	return &n
}

func (c *Client) WithToken(token string) *Client {
	n := *c
	n.Token = token
	return &n
}

func (c *Client) WithCookie(cookie *http.Cookie) *Client {
	n := *c
	n.Cookie = cookie
	return &n
}

func (c *Client) putJSON(ctx context.Context, path string, obj any, headerKV ...string) (*http.Request, *http.Response, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, nil, err
	}
	return c.doRequest(ctx, http.MethodPut, path, bytes.NewBuffer(data), append(headerKV, "Content-Type", "application/json")...)
}

func (c *Client) postJSON(ctx context.Context, path string, obj any, headerKV ...string) (*http.Request, *http.Response, error) {
	var body io.Reader

	switch v := obj.(type) {
	case string:
		if v != "" {
			body = strings.NewReader(v)
		}
	default:
		data, err := json.Marshal(obj)
		if err != nil {
			return nil, nil, err
		}
		body = bytes.NewBuffer(data)
		headerKV = append(headerKV, "Content-Type", "application/json")
	}
	return c.doRequest(ctx, http.MethodPost, path, body, headerKV...)
}

func (c *Client) doStream(ctx context.Context, method, path string, body io.Reader, headerKV ...string) (*http.Request, *http.Response, error) {
	return c.doRequest(ctx, method, path, body, append(headerKV, "Accept", "text/event-stream")...)
}

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader, headerKV ...string) (*http.Request, *http.Response, error) {
	if log.IsDebug() {
		var (
			data    = "[NONE]"
			headers string
		)
		if body != nil {
			dataBytes, err := io.ReadAll(body)
			if err != nil {
				return nil, nil, err
			}
			if utf8.Valid(dataBytes) {
				data = string(dataBytes)
			} else {
				data = fmt.Sprintf("[BINARY DATA len(%d)]", len(dataBytes))
			}

			body = bytes.NewReader(dataBytes)
		}
		// Convert headerKV... into a string of format k1=v1, k2=v2, ...
		for i := 0; i < len(headerKV); i += 2 {
			headers += fmt.Sprintf("%s=%s, ", headerKV[i], headerKV[i+1])
		}
		log.Fields("method", method, "path", path, "body", data, "headers", headers).Debugf("HTTP Request")
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, nil, err
	}

	if c.Token == "" && c.tokenFetcher != nil {
		token, err := c.tokenFetcher(ctx, c.BaseURL)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to fetch token: %w", err)
		}
		c.Token = token
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.Cookie != nil {
		req.AddCookie(c.Cookie)
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
		return nil, nil, &types.ErrHTTP{
			Code:    resp.StatusCode,
			Message: msg,
		}
	}
	if log.IsDebug() && !slices.Contains(headerKV, "text/event-stream") {
		var data string
		dataBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, nil, err
		}
		if utf8.Valid(dataBytes) {
			data = string(dataBytes)
		} else {
			data = fmt.Sprintf("[BINARY DATA len(%d)]", len(dataBytes))
		}
		log.Fields("method", method, "path", path, "body", data, "code", resp.StatusCode).Debugf("HTTP Response")
		resp.Body = io.NopCloser(bytes.NewReader(dataBytes))
	}
	return req, resp, err
}

func toStream[T any](resp *http.Response) chan T {
	ch := make(chan T)
	go func() {
		defer resp.Body.Close()
		defer close(ch)
		var eventName string
		lines := bufio.NewScanner(resp.Body)
		for lines.Scan() {
			var obj T
			if data, ok := strings.CutPrefix(lines.Text(), "data: "); ok && eventName == "" || eventName == "message" {
				if log.IsDebug() {
					log.Fields("data", data).Debugf("Received data")
				}
				if err := json.Unmarshal([]byte(data), &obj); err == nil {
					ch <- obj
				} else {
					errBytes, _ := json.Marshal(map[string]any{
						"error": err.Error(),
					})
					if err := json.Unmarshal(errBytes, &obj); err == nil {
						ch <- obj
					}
				}
			} else if event, ok := strings.CutPrefix(lines.Text(), "event: "); ok {
				if log.IsDebug() {
					log.Fields("event", event).Debugf("Received event")
				}
				eventName = event
			} else if strings.TrimSpace(lines.Text()) == "" {
				eventName = ""
			}
		}
	}()
	return ch
}

func toObject[T any](resp *http.Response, obj T) (def T, _ error) {
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(obj); err != nil {
		return def, err
	}
	return obj, nil
}

func (c *Client) runURLFromOpts(opts ListRunsOptions) string {
	url := "/runs"
	if opts.AgentID != "" && opts.ThreadID != "" {
		url = fmt.Sprintf("/agents/%s/threads/%s/runs", opts.AgentID, opts.ThreadID)
	} else if opts.AgentID != "" {
		url = fmt.Sprintf("/agents/%s/runs", opts.AgentID)
	} else if opts.ThreadID != "" {
		url = fmt.Sprintf("/threads/%s/runs", opts.ThreadID)
	}
	return url
}
