package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestMCPConnectCallbackPathFlags(t *testing.T) {
	connect := &MCPConnect{}
	cmd := &cobra.Command{}
	connect.Customize(cmd)
	require.NoError(t, cmd.ParseFlags([]string{"https://obot.example/mcp-connect/test", "--callback-path", "/first,callback", "--callback-path", "/second/callback"}))
	require.Equal(t, []string{"/first,callback", "/second/callback"}, connect.callbackPaths)
	require.NoError(t, cmd.Args(cmd, cmd.Flags().Args()))
}
