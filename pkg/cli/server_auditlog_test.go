package cli

import (
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type ServerCommand struct {
	Server
}

func (s *ServerCommand) Run(_ *cobra.Command, _ []string) error {
	return nil
}

func TestMCPAuditLogFlags(t *testing.T) {
	for _, tt := range []struct {
		name      string
		env       string
		args      []string
		want      *int
		wantError bool
	}{
		{
			name: "unset",
		},
		{
			name: "zero flag",
			args: []string{"--mcpaudit-log-max-body-bytes=0"},
			want: new(0),
		},
		{
			name: "positive flag",
			args: []string{"--mcpaudit-log-max-body-bytes=100"},
			want: new(100),
		},
		{
			name: "zero environment",
			env:  "0",
			want: new(0),
		},
		{
			name: "positive environment",
			env:  "200",
			want: new(200),
		},
		{
			name: "flag overrides environment",
			env:  "200",
			args: []string{"--mcpaudit-log-max-body-bytes=0"},
			want: new(0),
		},
		{
			name:      "invalid flag",
			args:      []string{"--mcpaudit-log-max-body-bytes=invalid"},
			wantError: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OBOT_SERVER_MCPAUDIT_LOG_MAX_BODY_BYTES", tt.env)
			t.Setenv("OBOT_SERVER_DISABLE_MCPAUDIT_LOG", "true")

			server := &ServerCommand{}
			root := cmd.Command(&Obot{}, server)
			root.PersistentPreRunE = nil
			root.SetArgs(append([]string{"server"}, tt.args...))

			err := root.Execute()
			if tt.wantError {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, server.MCPAuditLogMaxBodyBytes)
			require.True(t, server.DisableMCPAuditLog)
		})
	}
}
