package llmengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func testJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Existing fixtures assert JSON views; decode direct results without retaining an
// engine HTTP contract. Envelopes exist only for those compact fixture inputs.
func testResult(out any, envelope string) func(any, error) error {
	return func(value any, err error) error {
		if err != nil || out == nil {
			return err
		}
		if envelope != "" {
			value = map[string]any{envelope: value}
		}
		raw, err := json.Marshal(value)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, out)
	}
}

// Inference fixtures need their setup applied locally. Persistence regressions use
// direct operations instead so this helper cannot conceal an implicit runtime wait.
func testMutation(e *Engine, ctx context.Context, out any, envelope ...string) func(any, error) error {
	return func(value any, err error) error {
		if err == nil {
			_, _ = e.Sync(ctx)
		}
		key := ""
		if len(envelope) > 0 {
			key = envelope[0]
		}
		return testResult(out, key)(value, err)
	}
}

func testInference(t *testing.T, e *Engine, ctx context.Context, method, path string, body, out any) error {
	t.Helper()
	if !strings.HasPrefix(path, "/v1/") && !strings.HasPrefix(path, "/anthropic/") {
		return fmt.Errorf("test inference transport cannot administer %s", path)
	}
	var raw []byte
	if body != nil {
		raw = testJSON(t, body)
	}
	u := e.BaseURL()
	req, err := http.NewRequestWithContext(ctx, method, u.String()+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	resp, err := e.Transport().RoundTrip(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return &APIError{StatusCode: resp.StatusCode, Message: string(raw)}
	}
	if out == nil {
		_, err := io.Copy(io.Discard, resp.Body)
		return err
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
