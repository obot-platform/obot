package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestCollectAllFollowsPagination(t *testing.T) {
	ctx := t.Context()

	server := gomcp.NewServer(&gomcp.Implementation{Name: "paginated", Version: "1.0.0"}, &gomcp.ServerOptions{PageSize: 2})
	for i := range 5 {
		gomcp.AddTool(server, &gomcp.Tool{Name: fmt.Sprintf("tool_%d", i)},
			func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, any, error) {
				return &gomcp.CallToolResult{}, nil, nil
			})
		server.AddPrompt(&gomcp.Prompt{Name: fmt.Sprintf("prompt_%d", i)},
			func(context.Context, *gomcp.GetPromptRequest) (*gomcp.GetPromptResult, error) {
				return &gomcp.GetPromptResult{}, nil
			})
		server.AddResource(&gomcp.Resource{Name: fmt.Sprintf("resource_%d", i), URI: fmt.Sprintf("test://resource/%d", i)},
			func(context.Context, *gomcp.ReadResourceRequest) (*gomcp.ReadResourceResult, error) {
				return &gomcp.ReadResourceResult{}, nil
			})
	}

	serverTransport, clientTransport := gomcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Close() })

	client := gomcp.NewClient(&gomcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	// Sanity check: a single list call only returns the first page.
	firstPage, err := session.ListTools(ctx, &gomcp.ListToolsParams{})
	require.NoError(t, err)
	require.Len(t, firstPage.Tools, 2)
	require.NotEmpty(t, firstPage.NextCursor)

	tools, err := collectAll(session.Tools(ctx, nil))
	require.NoError(t, err)
	require.Len(t, tools, 5)
	for i, tool := range tools {
		require.Equal(t, fmt.Sprintf("tool_%d", i), tool.Name)
	}

	prompts, err := collectAll(session.Prompts(ctx, nil))
	require.NoError(t, err)
	require.Len(t, prompts, 5)

	resources, err := collectAll(session.Resources(ctx, nil))
	require.NoError(t, err)
	require.Len(t, resources, 5)
}

func TestCollectAllReturnsError(t *testing.T) {
	wantErr := errors.New("list failed")
	seq := func(yield func(*gomcp.Tool, error) bool) {
		if !yield(&gomcp.Tool{Name: "first"}, nil) {
			return
		}
		yield(nil, wantErr)
	}

	tools, err := collectAll(seq)
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, tools)
}
