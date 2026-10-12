package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/obot-platform/obot/apiclient/types"
)

// enabledModelProxySettings reports that the model proxy is enabled, as an installation that never changed the setting
// does, without a database.
type enabledModelProxySettings struct{}

type testerDeadlineReader struct {
	ctx context.Context
}

func (r testerDeadlineReader) Read([]byte) (int, error) {
	<-r.ctx.Done()

	return 0, r.ctx.Err()
}

func (enabledModelProxySettings) ModelProxyEnabled(context.Context) (bool, error) {
	return true, nil
}

func TestTesterModelProxyTimeout(t *testing.T) {
	for _, tt := range []struct {
		name      string
		streaming bool
		status    int
	}{
		{
			name:   "waiting for response headers",
			status: http.StatusGatewayTimeout,
		},
		{
			name:      "during streaming",
			streaming: true,
			status:    http.StatusOK,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.NotFoundHandler())
			defer upstream.Close()

			handler := newModelProxyTestHandler(t, &fakeTesterProviders{}, &fakeTesterLicense{key: "license"}, upstream)
			// The chat runs in a synctest bubble, where reading a database that goroutines outside the bubble share,
			// such as the gateway client's background work, aborts the test binary.
			handler.modelProxy.Settings = enabledModelProxySettings{}
			handler.modelProxyClient = &http.Client{Transport: mcpTesterRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				if !tt.streaming {
					<-r.Context().Done()

					return nil, r.Context().Err()
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": {"text/event-stream"}},
					Body:       io.NopCloser(testerDeadlineReader{ctx: r.Context()}),
				}, nil
			})}

			// Advance the actual server-owned deadline without waiting ten minutes.
			synctest.Test(t, func(t *testing.T) {
				response := runMCPTesterChat(t, handler, "user-1", modelProxyChatBody)
				if response.Code != tt.status {
					t.Fatalf("status=%d body=%s", response.Code, response.Body)
				}

				data := response.Body.String()
				if tt.streaming {
					data = strings.TrimSpace(strings.TrimPrefix(data, "data: "))
				}

				var payload types.MCPTesterErrorResponse
				if err := json.Unmarshal([]byte(data), &payload); err != nil {
					t.Fatal(err)
				}

				if payload.Error.Code != types.MCPTesterErrorProvider || !payload.Error.Retryable || !strings.Contains(payload.Error.Message, "timed out") {
					t.Fatalf("incorrect timeout: %#v", payload.Error)
				}
			})
		})
	}
}
