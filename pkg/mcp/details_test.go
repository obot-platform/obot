package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

type inspectionBackend struct {
	backend
	running     bool
	lookupErr   error
	deployments int
	logStreams  int
}

func (b *inspectionBackend) getServerDetails(context.Context, string) (types.MCPServerDetails, error) {
	if b.lookupErr != nil {
		return types.MCPServerDetails{}, b.lookupErr
	}
	if !b.running {
		return types.MCPServerDetails{}, fmt.Errorf("lookup: %w", ErrServerNotRunning)
	}
	return types.MCPServerDetails{IsAvailable: true, ReadyReplicas: 1}, nil
}

func (b *inspectionBackend) deployServer(context.Context, ServerConfig) error {
	b.deployments++
	b.running = true
	return nil
}

func (b *inspectionBackend) streamServerLogs(context.Context, string) (io.ReadCloser, error) {
	b.logStreams++
	return io.NopCloser(strings.NewReader("existing logs")), nil
}

func TestServerInspectionDeployment(t *testing.T) {
	lookupErr := errors.New("backend unavailable")
	for _, operation := range []string{"details", "logs"} {
		for _, tc := range []struct {
			name        string
			readOnly    bool
			running     bool
			lookupErr   error
			wantErr     error
			deployments int
		}{
			{
				name:     "read-only stopped server",
				readOnly: true,
				wantErr:  ErrServerNotRunning,
			},
			{
				name:     "read-only running server",
				readOnly: true,
				running:  true,
			},
			{
				name:        "writable stopped server",
				deployments: 1,
			},
			{
				name:    "writable running server",
				running: true,
			},
			{
				name:      "read-only backend failure",
				readOnly:  true,
				lookupErr: lookupErr,
				wantErr:   lookupErr,
			},
			{
				name:      "writable backend failure",
				lookupErr: lookupErr,
				wantErr:   lookupErr,
			},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				backend := &inspectionBackend{running: tc.running, lookupErr: tc.lookupErr}
				manager := &SessionManager{backend: backend}
				config := ServerConfig{MCPServerName: "server", Runtime: types.RuntimeContainerized}
				opts := ServerInspectionOptions{ReadOnly: tc.readOnly}
				var err error
				if operation == "details" {
					var details types.MCPServerDetails
					details, err = manager.GetServerDetails(t.Context(), config, opts)
					if tc.wantErr == nil {
						require.True(t, details.IsAvailable)
					}
				} else {
					var logs io.ReadCloser
					logs, err = manager.StreamServerLogs(t.Context(), config, opts)
					if tc.wantErr == nil {
						require.NoError(t, err)
						data, readErr := io.ReadAll(logs)
						require.NoError(t, logs.Close())
						require.NoError(t, readErr)
						require.Equal(t, "existing logs", string(data))
						require.Equal(t, 1, backend.logStreams)
					} else {
						require.Nil(t, logs)
						require.Zero(t, backend.logStreams)
					}
				}
				if tc.wantErr == nil {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, tc.wantErr)
				}
				require.Equal(t, tc.deployments, backend.deployments)
			})
		}
	}
}
