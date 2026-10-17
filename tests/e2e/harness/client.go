//go:build e2e

package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Client calls one Obot replica as one principal: a browser session (cookie jar) or an API
// key. Session cookies are host-scoped, not port-scoped, so a session works on every replica.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// Response is a buffered HTTP response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

func (r *Response) String() string {
	return fmt.Sprintf("HTTP %d: %s", r.Status, truncate(string(r.Body), 2000))
}

// JSON decodes the body into v.
func (r *Response) JSON(tb testing.TB, v any) {
	tb.Helper()
	if err := json.Unmarshal(r.Body, v); err != nil {
		tb.Fatalf("decode response body: %v\n%s", err, r)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}

// NewSessionClient returns a client with an empty cookie jar.
func NewSessionClient(baseURL string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		BaseURL: baseURL,
		HTTP: &http.Client{
			Jar:     jar,
			Timeout: 2 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// NewAPIKeyClient returns a client that authenticates with an API key.
func NewAPIKeyClient(baseURL, key string) *Client {
	return &Client{BaseURL: baseURL, APIKey: key, HTTP: &http.Client{Timeout: 2 * time.Minute}}
}

// On returns a copy of the client that calls another replica as the same principal.
func (c *Client) On(baseURL string) *Client {
	cp := *c
	cp.BaseURL = baseURL
	return &cp
}

// Request sends a request. body may be nil, []byte, string or a value to encode as JSON.
func (c *Client) Request(ctx context.Context, method, path string, body any, headers map[string]string) (*Response, error) {
	var reader io.Reader
	contentType := ""
	switch b := body.(type) {
	case nil:
	case []byte:
		reader = bytes.NewReader(b)
		contentType = "application/json"
	case string:
		reader = strings.NewReader(b)
		contentType = "application/json"
	default:
		data, err := json.Marshal(b)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
		contentType = "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &Response{Status: resp.StatusCode, Header: resp.Header, Body: data}, nil
}

// Do sends a request and fails the test on a transport error.
func (c *Client) Do(tb testing.TB, method, path string, body any, headers ...map[string]string) *Response {
	tb.Helper()
	var h map[string]string
	if len(headers) > 0 {
		h = headers[0]
	}
	resp, err := c.Request(context.Background(), method, path, body, h)
	if err != nil {
		tb.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// Expect sends a request and fails the test unless it gets the wanted status. It decodes a
// JSON body into out when out is non-nil.
func (c *Client) Expect(tb testing.TB, want int, method, path string, body, out any) *Response {
	tb.Helper()
	resp := c.Do(tb, method, path, body)
	if resp.Status != want {
		tb.Fatalf("%s %s%s: want HTTP %d, got %s", method, c.BaseURL, path, want, resp)
	}
	if out != nil {
		resp.JSON(tb, out)
	}
	return resp
}

// Get expects 200 and decodes the body into out.
func (c *Client) Get(tb testing.TB, path string, out any) {
	tb.Helper()
	c.Expect(tb, http.StatusOK, http.MethodGet, path, nil, out)
}

// Post sends JSON and expects a 2xx status, decoding the body into out.
func (c *Client) Post(tb testing.TB, path string, body, out any) *Response {
	tb.Helper()
	return c.expect2xx(tb, http.MethodPost, path, body, out)
}

// Put sends JSON and expects a 2xx status, decoding the body into out.
func (c *Client) Put(tb testing.TB, path string, body, out any) *Response {
	tb.Helper()
	return c.expect2xx(tb, http.MethodPut, path, body, out)
}

// Delete expects a 2xx status.
func (c *Client) Delete(tb testing.TB, path string) {
	tb.Helper()
	c.expect2xx(tb, http.MethodDelete, path, nil, nil)
}

func (c *Client) expect2xx(tb testing.TB, method, path string, body, out any) *Response {
	tb.Helper()
	resp := c.Do(tb, method, path, body)
	if resp.Status < 200 || resp.Status >= 300 {
		tb.Fatalf("%s %s%s: want 2xx, got %s", method, c.BaseURL, path, resp)
	}
	if out != nil && len(resp.Body) > 0 {
		resp.JSON(tb, out)
	}
	return resp
}

// ActivateOwner sets the initial local-auth owner's password with its setup token.
func ActivateOwner(ctx context.Context, baseURL, setupToken, password string) error {
	c := NewSessionClient(baseURL)
	resp, err := c.Request(ctx, http.MethodPost, "/api/local-auth/activate", map[string]string{"setupToken": setupToken}, nil)
	if err != nil {
		return err
	}
	if resp.Status != http.StatusOK {
		return fmt.Errorf("activate owner: %s", resp)
	}
	resp, err = c.Request(ctx, http.MethodPost, "/api/local-auth/change-password", map[string]string{"password": password}, nil)
	if err != nil {
		return err
	}
	if resp.Status != http.StatusOK {
		return fmt.Errorf("set owner password: %s", resp)
	}
	return nil
}

// Login signs in with local auth and returns a session client.
func Login(ctx context.Context, baseURL, email, password string) (*Client, error) {
	c := NewSessionClient(baseURL)
	if _, err := c.Request(ctx, http.MethodGet, "/oauth2/start?rd=/", nil, nil); err != nil {
		return nil, err
	}
	form := url.Values{"email": {email}, "password": {password}, "rd": {"/"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/oauth2/start", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	me, err := c.Request(ctx, http.MethodGet, "/api/me", nil, nil)
	if err != nil {
		return nil, err
	}
	if me.Status != http.StatusOK || strings.Contains(string(me.Body), `"username":"anonymous"`) || !strings.Contains(string(me.Body), email) {
		return nil, fmt.Errorf("login as %s failed (login HTTP %d): /api/me %s", email, resp.StatusCode, me)
	}
	return c, nil
}

// Eventually polls check until it returns nil or the timeout passes, then fails the test
// with the last error.
func Eventually(tb testing.TB, timeout time.Duration, what string, check func() error) {
	tb.Helper()
	start := time.Now()
	deadline := start.Add(timeout)
	interval := 100 * time.Millisecond
	for {
		err := check()
		if err == nil {
			if took := time.Since(start); took > 10*time.Second {
				tb.Logf("waiting for %s took %s", what, took.Round(time.Millisecond))
			}
			return
		}
		if time.Now().After(deadline) {
			tb.Fatalf("timed out after %s waiting for %s: %v", timeout, what, err)
		}
		time.Sleep(interval)
		if interval < time.Second {
			interval += 100 * time.Millisecond
		}
	}
}
