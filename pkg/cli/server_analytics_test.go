package cli

import (
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/pkg/producttelemetry"
	"github.com/spf13/cobra"
)

func TestProductAnalyticsModeEnvironment(t *testing.T) {
	t.Setenv("OBOT_SERVER_PRODUCT_ANALYTICS_MODE", "off")
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
	if server.ProductAnalyticsMode != producttelemetry.ModeOff {
		t.Fatalf("product analytics mode = %q, want off", server.ProductAnalyticsMode)
	}
}
