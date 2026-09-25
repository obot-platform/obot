package cli

import (
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/spf13/cobra"
)

func TestDisableProductAnalyticsEnvironment(t *testing.T) {
	t.Setenv("OBOT_SERVER_DISABLE_PRODUCT_ANALYTICS", "true")
	server := &Server{}
	command := cmd.Command(&Obot{}, server)
	command.PersistentPreRunE = nil
	serverCommand := command.Commands()[0]
	serverCommand.RunE = nil
	serverCommand.Run = func(_ *cobra.Command, _ []string) {}
	command.SetArgs([]string{"server"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !server.DisableProductAnalytics {
		t.Fatal("OBOT_SERVER_DISABLE_PRODUCT_ANALYTICS=true did not enable the setting")
	}
}
