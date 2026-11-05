package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/require"
)

func TestTesterAuditLargeTerminalEvent(t *testing.T) {
	for _, tc := range []struct {
		name     string
		limit    int
		textSize int
		newline  string
		wantBody bool
	}{
		{
			name:     "terminal event above one MiB",
			limit:    defaultLLMAuditLogResponseCaptureLimit,
			textSize: 1200 << 10,
			newline:  "\n\n",
			wantBody: true,
		},
		{
			name:     "terminal event ending at EOF",
			limit:    defaultLLMAuditLogResponseCaptureLimit,
			textSize: 1200 << 10,
			wantBody: true,
		},
		{
			name:     "custom capture limit bounds pending event",
			limit:    64 << 10,
			textSize: 96 << 10,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{
				"id":     "resp-large",
				"status": "completed",
				"output": []any{map[string]any{
					"type": "message",
					"content": []any{map[string]any{
						"type": "output_text",
						"text": strings.Repeat("x", tc.textSize),
					}},
				}},
				"usage": map[string]int{
					"input_tokens":  42,
					"output_tokens": 7,
				},
			})
			require.NoError(t, err)
			stream := `data: {"type":"response.completed","response":` + string(body) + `}` + tc.newline
			recorder := newLLMAuditRecorder(httptest.NewRequest(http.MethodPost, "/tester/chat", nil), nil, tc.limit)
			recorder.recordResponse(&http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/event-stream"}},
			})
			reader := &testerAuditReader{
				Reader:   iotest.HalfReader(strings.NewReader(stream)),
				recorder: recorder,
			}

			forwarded, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, stream, string(forwarded), "audit capture must not alter the client stream")
			require.LessOrEqual(t, len(reader.pending), tc.limit)
			recorder.finishBody()
			if !tc.wantBody {
				require.True(t, recorder.responseCaptureFilled)
				require.Empty(t, recorder.log.ResponseBody)
				return
			}
			require.NotEmpty(t, recorder.log.ResponseBody)
			require.Equal(t, string(body), string(recorder.log.ResponseBody))
			require.Equal(t, "resp-large", recorder.log.ResponseID)
			require.Equal(t, 42, recorder.log.InputTokens)
			require.Equal(t, 7, recorder.log.OutputTokens)
		})
	}
}
