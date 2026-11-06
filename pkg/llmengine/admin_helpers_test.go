package llmengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
func testMutation(ctx context.Context, e *Engine, out any, envelope ...string) func(any, error) error {
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

func testInference(ctx context.Context, t *testing.T, e *Engine, method, path string, body, out any) error {
	t.Helper()
	if !strings.HasPrefix(path, "/v1/") && !strings.HasPrefix(path, "/anthropic/") {
		return fmt.Errorf("test inference transport cannot administer %s", path)
	}
	var raw []byte
	if body != nil {
		raw = testJSON(t, body)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, req)
	if recorder.Code >= 300 {
		return &apierrors.StatusError{ErrStatus: metav1.Status{Code: int32(recorder.Code), Message: recorder.Body.String()}}
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(recorder.Body.Bytes(), out)
}
