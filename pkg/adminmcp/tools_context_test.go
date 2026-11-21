package adminmcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/stretchr/testify/require"
)

func TestToolContextCancellation(t *testing.T) {
	tests := []struct {
		name            string
		cancelCarrier   bool
		alreadyCanceled bool
	}{
		{
			name:          "carrier request canceled during call",
			cancelCarrier: true,
		},
		{
			name: "MCP call canceled during call",
		},
		{
			name:            "carrier request already canceled",
			cancelCarrier:   true,
			alreadyCanceled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			carrier, cancelCarrier := context.WithCancel(t.Context())
			defer cancelCarrier()
			call, cancelCall := context.WithCancel(t.Context())
			defer cancelCall()
			call = context.WithValue(call, requestKey{}, api.Context{
				Request: httptest.NewRequest(http.MethodPost, "/", nil).WithContext(carrier),
			})
			if tt.alreadyCanceled {
				cancelCarrier()
			}
			type workContexts struct {
				tool context.Context
				api  context.Context
			}
			started := make(chan workContexts, 1)
			finished := make(chan error, 1)
			handler := bind(&Handler{}, func(tools *tools, ctx context.Context, _ *gomcp.CallToolRequest, _ listCatalogsInput) (*gomcp.CallToolResult, listCatalogsOutput, error) {
				started <- workContexts{tool: ctx, api: tools.req.Context()}
				<-ctx.Done()
				return nil, listCatalogsOutput{}, ctx.Err()
			})
			go func() {
				_, _, err := handler(call, nil, listCatalogsInput{})
				finished <- err
			}()
			var work workContexts
			select {
			case work = <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("tool did not start")
			}
			if tt.cancelCarrier {
				cancelCarrier()
			} else {
				cancelCall()
			}
			select {
			case err := <-finished:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(5 * time.Second):
				t.Fatal("tool did not stop")
			}
			require.ErrorIs(t, work.tool.Err(), context.Canceled)
			require.ErrorIs(t, work.api.Err(), context.Canceled)
		})
	}
}
