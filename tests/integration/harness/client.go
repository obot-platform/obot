//go:build integration

package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

const requestTimeout = 2 * time.Minute

// do issues an HTTP request to BaseURL+path and decodes a JSON response into
// `out` if non-nil. It fails the test on transport errors or non-2xx
// responses, with the response body included in the failure message.
func (h *Harness) do(t *testing.T, method, path string, body, out any) {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s: %v", method, path, err)
		}
		reqBody = bytes.NewReader(buf)
	}

	ctx, cancel := context.WithTimeout(t.Context(), requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, h.BaseURL+path, reqBody)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		t.Fatalf("%s %s: %d %s\nbody: %s", method, path, resp.StatusCode, resp.Status, string(respBody))
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			t.Fatalf("decode %s %s: %v\nbody: %s", method, path, err, string(respBody))
		}
	}
}

// Get issues GET path and decodes into out.
func (h *Harness) Get(t *testing.T, path string, out any) {
	t.Helper()
	h.do(t, http.MethodGet, path, nil, out)
}

// Post issues POST path with body, decoding into out.
func (h *Harness) Post(t *testing.T, path string, body, out any) {
	t.Helper()
	h.do(t, http.MethodPost, path, body, out)
}

// Delete issues DELETE path.
func (h *Harness) Delete(t *testing.T, path string) {
	t.Helper()
	h.do(t, http.MethodDelete, path, nil, nil)
}

// Status issues a request and returns the status code without failing the
// test on non-2xx. Useful for asserting expected-error responses (e.g.
// "should return 404 after delete").
func (h *Harness) Status(t *testing.T, method, path string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), requestTimeout)
	defer cancel()
	status, err := h.status(ctx, method, path)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return status
}

func (h *Harness) status(ctx context.Context, method, path string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, h.BaseURL+path, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
