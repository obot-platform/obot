package cli

import (
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/spf13/cobra"
)

func TestBillingConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		key     string
		args    []string
		wantURL string
		wantKey string
	}{
		{
			name: "unset",
		},
		{
			name:    "environment",
			url:     "https://billing.obot.ai",
			key:     "billing-key",
			wantURL: "https://billing.obot.ai",
			wantKey: "billing-key",
		},
		{
			name:    "flags override environment",
			url:     "https://billing.obot.ai",
			key:     "billing-key",
			args:    []string{"--billing-url=https://billing.example.com", "--billing-key=other-key"},
			wantURL: "https://billing.example.com",
			wantKey: "other-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OBOT_SERVER_BILLING_URL", tt.url)
			t.Setenv("OBOT_SERVER_BILLING_KEY", tt.key)

			server := &Server{}
			command := cmd.Command(&Obot{}, server)
			command.PersistentPreRunE = nil
			serverCommand := command.Commands()[0]
			serverCommand.RunE = nil
			serverCommand.Run = func(_ *cobra.Command, _ []string) {}
			command.SetArgs(append([]string{"server"}, tt.args...))
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}

			if server.BillingURL != tt.wantURL {
				t.Fatalf("billing URL = %q, want %q", server.BillingURL, tt.wantURL)
			}
			if server.BillingKey != tt.wantKey {
				t.Fatalf("billing key = %q, want %q", server.BillingKey, tt.wantKey)
			}
		})
	}
}
