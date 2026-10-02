package cli

import (
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/stretchr/testify/require"
)

func TestMCPLoginFlags(t *testing.T) {
	login := &MCPLogin{}
	command := cmd.Command(login)
	require.NoError(t, command.ParseFlags([]string{"--url", "https://obot.example/mcp-connect/test", "--callback-path", "/custom/callback"}))
	require.Equal(t, "https://obot.example/mcp-connect/test", login.URL)
	require.Equal(t, []string{"/custom/callback"}, login.callbackPaths)
	require.NoError(t, command.Args(command, command.Flags().Args()))
	require.Error(t, command.Args(command, []string{"unexpected"}))
}

func TestMCPLoginRequiresURL(t *testing.T) {
	login := &MCPLogin{}
	require.ErrorContains(t, login.Run(cmd.Command(login), nil), "--url is required")
}
