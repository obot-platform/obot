package cli

import (
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/stretchr/testify/require"
)

func TestMCPLoginArgument(t *testing.T) {
	login := &MCPLogin{}
	command := cmd.Command(login)
	require.NoError(t, command.Args(command, []string{"https://obot.example/oauth/mcp/login/attempt"}))
	require.Error(t, command.Args(command, nil))
	require.Error(t, command.Args(command, []string{"one", "two"}))
}

func TestMCPLoginRequiresURL(t *testing.T) {
	login := &MCPLogin{}
	require.ErrorContains(t, login.Run(cmd.Command(login), nil), "authentication URL is required")
	require.ErrorContains(t, login.Run(cmd.Command(login), []string{"  "}), "authentication URL is required")
}

func TestMCPAuthenticationCommands(t *testing.T) {
	command := cmd.Command(&MCP{})
	login, _, err := command.Find([]string{"login"})
	require.NoError(t, err)
	require.Equal(t, "login", login.Name())
	for _, child := range command.Commands() {
		require.NotEqual(t, "connect", child.Name())
	}
}
