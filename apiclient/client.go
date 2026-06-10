package apiclient

import (
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

type tokenFetcher func(context.Context, string, TokenFetchOptions) (string, error)

type TokenFetchOptions struct {
	Name         string
	Description  string
	NoExpiration bool
	ForceRefresh bool
	Scopes       []string
}

type Client struct {
	BaseURL      string
	Token        string
	Cookie       *http.Cookie
	tokenFetcher tokenFetcher
}

func (c *Client) WithTokenFetcher(f tokenFetcher) *Client {
	n := *c
	n.tokenFetcher = f
	return &n
}

func (c *Client) WithToken(token string) *Client {
	n := *c
	n.Token = token
	return &n
}

func (c *Client) GetToken(ctx context.Context, opts TokenFetchOptions) (string, error) {
	if c.Token != "" && !opts.ForceRefresh {
		return c.Token, nil
	}
	if c.tokenFetcher != nil {
		return c.tokenFetcher(ctx, c.BaseURL, opts)
	}
	return "", fmt.Errorf("no token or token fetcher")
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

func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader, headerKV ...string) (*http.Request, *http.Response, error) {
	return c.doRequestWithBaseURL(ctx, method, c.BaseURL, path, body, headerKV...)
}

func (c *Client) doRequestWithBaseURL(ctx context.Context, method, baseURL, path string, body io.Reader, headerKV ...string) (*http.Request, *http.Response, error) {
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

	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, body)
	if err != nil {
		return nil, nil, err
	}

	if c.Token == "" && c.tokenFetcher != nil {
		token, err := c.GetToken(ctx, TokenFetchOptions{
			Name:   "CLI Token",
			Scopes: []string{"api"}, // Default to requesting a token with API access.
		})
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

func toObject[T any](resp *http.Response, obj T) (def T, _ error) {
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(obj); err != nil {
		return def, err
	}
	return obj, nil
}
