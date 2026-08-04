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

	"github.com/klauspost/compress/zstd"
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
	if !opts.ForceRefresh && c.Token != "" {
		return c.Token, TokenHasScopes(ctx, c.BaseURL, c.Token, opts.Scopes)
	}
	if c.tokenFetcher != nil {
		return c.tokenFetcher(ctx, c.BaseURL, opts)
	}
	return "", fmt.Errorf("no token or token fetcher")
}

type tokenScopeValidationResponse struct {
	Allowed bool `json:"allowed"`
	Scopes  struct {
		CanAccessAPI                bool     `json:"canAccessAPI"`
		CanAccessSkills             bool     `json:"canAccessSkills"`
		CanAccessLLMProxy           bool     `json:"canAccessLLMProxy"`
		CanAccessPublishedArtifacts bool     `json:"canAccessPublishedArtifacts"`
		CanAccessDeviceScans        bool     `json:"canAccessDeviceScans"`
		MCPServerIDs                []string `json:"mcpServerIds,omitempty"`
	} `json:"scopes"`
}

// TokenHasScopes reports whether token is valid for baseURL and includes all requested scopes.
func TokenHasScopes(ctx context.Context, baseURL, token string, scopes []string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+"/api-keys/auth", strings.NewReader(`{"validateOnly": true}`))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var validation tokenScopeValidationResponse
	if err := json.NewDecoder(resp.Body).Decode(&validation); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	if !validation.Allowed {
		return fmt.Errorf("token is not allowed")
	}

	for _, scope := range scopes {
		switch scope {
		case types.APIKeyScopeAPI:
			if !validation.Scopes.CanAccessAPI {
				return fmt.Errorf("token does not have scope: %s", scope)
			}
		case types.APIKeyScopeSkills:
			if !validation.Scopes.CanAccessSkills && !validation.Scopes.CanAccessAPI {
				return fmt.Errorf("token does not have scope: %s", scope)
			}
		case types.APIKeyScopeLLM:
			if !validation.Scopes.CanAccessLLMProxy && !validation.Scopes.CanAccessAPI {
				return fmt.Errorf("token does not have scope: %s", scope)
			}
		case types.APIKeyScopePublishedArtifacts:
			if !validation.Scopes.CanAccessPublishedArtifacts && !validation.Scopes.CanAccessAPI {
				return fmt.Errorf("token does not have scope: %s", scope)
			}
		case types.APIKeyScopeAllMCP:
			if !slices.Contains(validation.Scopes.MCPServerIDs, "*") {
				return fmt.Errorf("token does not have scope: %s", scope)
			}
		case types.APIKeyScopeDeviceScans:
			if !validation.Scopes.CanAccessDeviceScans {
				return fmt.Errorf("token does not have scope: %s", scope)
			}
		default:
			return fmt.Errorf("unknown scope: %s", scope)
		}
	}

	return nil
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

// postCompressedJSON marshals obj and POSTs it zstd-compressed. Intended for
// bodies big enough to be worth compressing on every call. The server must
// decode Content-Encoding; there is no uncompressed fallback.
func (c *Client) postCompressedJSON(ctx context.Context, path string, obj any, headerKV ...string) (*http.Request, *http.Response, error) {
	data, err := json.Marshal(obj)
	if err != nil {
		return nil, nil, err
	}

	// Concurrency 1: no worker per CPU for a one-shot compression.
	encoder, err := zstd.NewWriter(nil,
		zstd.WithEncoderLevel(zstd.SpeedBetterCompression),
		zstd.WithEncoderConcurrency(1))
	if err != nil {
		return nil, nil, err
	}
	defer encoder.Close()

	return c.doRequest(ctx, http.MethodPost, path, bytes.NewReader(encoder.EncodeAll(data, nil)),
		slices.Concat(headerKV, []string{"Content-Type", "application/json", "Content-Encoding", "zstd"})...)
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
			Scopes: types.DefaultCLIAPIKeyScopes(),
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
		return nil, nil, errFromResponse(resp)
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

// maxErrorBodyBytes bounds how much of a non-2xx response body reaches an
// ErrHTTP message.
//
// The body is not necessarily an API error document. A wrong base path, a
// reverse proxy, or a load balancer answers with a whole HTML page, and callers
// put these messages in front of people and models: obot-sentry's enforcement
// hook renders one into the text it hands the agent, where an unbounded body
// buried the instructions after it. 4 KiB holds every error this API produces.
const maxErrorBodyBytes = 4 << 10

// errFromResponse builds the error for a non-2xx response and closes its body.
// Callers get no response on this path, so nothing else can close it.
func errFromResponse(resp *http.Response) error {
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes+1))
	truncated := len(data) > maxErrorBodyBytes
	if truncated {
		// A cut can split the last rune. Dropping the partial tail keeps a
		// truncated text body from being reported as a binary one below.
		data = trimPartialRune(data[:maxErrorBodyBytes])
	}

	if !utf8.Valid(data) {
		return &types.ErrHTTP{
			Code:    resp.StatusCode,
			Message: fmt.Sprintf("[non-text response body, %d bytes read]", len(data)),
		}
	}

	msg := strings.TrimSpace(string(data))
	switch {
	case msg == "":
		msg = resp.Status
	case truncated:
		msg += "…"
	}
	return &types.ErrHTTP{Code: resp.StatusCode, Message: msg}
}

// trimPartialRune removes an incomplete UTF-8 sequence from the end of data.
func trimPartialRune(data []byte) []byte {
	for range utf8.UTFMax - 1 {
		if len(data) == 0 {
			return data
		}
		if r, size := utf8.DecodeLastRune(data); r != utf8.RuneError || size > 1 {
			return data
		}
		data = data[:len(data)-1]
	}
	return data
}

func toObject[T any](resp *http.Response, obj T) (def T, _ error) {
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(obj); err != nil {
		return def, err
	}
	return obj, nil
}
