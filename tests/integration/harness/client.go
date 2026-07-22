//go:build integration

package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

// do issues an HTTP request to BaseURL+path and decodes a JSON response into
// `out` if non-nil. It fails the test on transport errors or non-2xx
// responses, with the response body included in the failure message.
func (h *Harness) do(ctx context.Context, method, path string, body, out any) {
	h.T.Helper()

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			h.T.Fatalf("marshal %s %s: %v", method, path, err)
		}
		reqBody = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, method, h.BaseURL+path, reqBody)
	if err != nil {
		h.T.Fatalf("build %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		h.T.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.T.Fatalf("%s %s: %d %s\nbody: %s", method, path, resp.StatusCode, resp.Status, string(respBody))
	}

	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			h.T.Fatalf("decode %s %s: %v\nbody: %s", method, path, err, string(respBody))
		}
	}
}

// Get issues GET path and decodes into out.
func (h *Harness) Get(ctx context.Context, path string, out any) {
	h.do(ctx, http.MethodGet, path, nil, out)
}

// Post issues POST path with body, decoding into out.
func (h *Harness) Post(ctx context.Context, path string, body, out any) {
	h.do(ctx, http.MethodPost, path, body, out)
}

// Put issues PUT path with body, decoding into out.
func (h *Harness) Put(ctx context.Context, path string, body, out any) {
	h.do(ctx, http.MethodPut, path, body, out)
}

// Delete issues DELETE path.
func (h *Harness) Delete(ctx context.Context, path string) {
	h.do(ctx, http.MethodDelete, path, nil, nil)
}

// ReadStreamUntil issues GET path against a streaming endpoint (e.g. SSE logs)
// and returns as soon as it reads expected, reaches maxBytes, or exhausts budget.
func (h *Harness) ReadStreamUntil(ctx context.Context, path string, expected []byte, budget time.Duration, maxBytes int) []byte {
	h.T.Helper()
	if maxBytes <= 0 {
		h.T.Fatalf("read stream %s: maxBytes must be positive", path)
	}

	streamCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, h.BaseURL+path, nil)
	if err != nil {
		h.T.Fatalf("build GET %s: %v", path, err)
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		h.T.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		h.T.Fatalf("GET %s: %d %s\nbody: %s", path, resp.StatusCode, resp.Status, string(body))
	}

	var result []byte
	chunk := make([]byte, min(1024, maxBytes))
	for {
		n, err := resp.Body.Read(chunk)
		if n > 0 {
			result = append(result, chunk[:min(n, maxBytes-len(result))]...)
			if bytes.Contains(result, expected) || len(result) == maxBytes {
				return result
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.EOF) {
			return result
		}
		h.T.Fatalf("read stream %s: %v", path, err)
	}
}

// Status issues a request and returns the status code without failing the
// test on non-2xx. Useful for asserting expected-error responses (e.g.
// "should return 404 after delete").
func (h *Harness) Status(ctx context.Context, method, path string) int {
	h.T.Helper()
	req, err := http.NewRequestWithContext(ctx, method, h.BaseURL+path, nil)
	if err != nil {
		h.T.Fatalf("build %s %s: %v", method, path, err)
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		h.T.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}
