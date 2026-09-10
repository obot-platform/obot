package cli

import (
	"os"
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/pkg/mcptester"
	"github.com/spf13/cobra"
)

func TestModelProxyURLConfiguration(t *testing.T) {
	tests := []struct {
		name string
		env  *string
		args []string
		want string
	}{
		{
			name: "unset default",
			want: mcptester.DefaultModelProxyURL,
		},
		{
			name: "empty disables",
			env:  new(""),
			want: "",
		},
		{
			name: "environment override",
			env:  new("https://custom.example"),
			want: "https://custom.example",
		},
		{
			name: "flag overrides empty environment",
			env:  new(""),
			args: []string{"--model-proxy-url=https://flag.example"},
			want: "https://flag.example",
		},
		{
			name: "empty flag overrides environment",
			env:  new("https://custom.example"),
			args: []string{"--model-proxy-url="},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OBOT_SERVER_MODEL_PROXY_URL", "")
			if tt.env == nil {
				if err := os.Unsetenv("OBOT_SERVER_MODEL_PROXY_URL"); err != nil {
					t.Fatal(err)
				}
			} else {
				t.Setenv("OBOT_SERVER_MODEL_PROXY_URL", *tt.env)
			}

			server := &Server{}
			command := cmd.Command(server)

			// Exercise the real flag/environment bindings and Server.Pre without
			// starting the application or contacting an external service.
			command.RunE = nil
			command.Run = func(_ *cobra.Command, _ []string) {}
			command.SetArgs(tt.args)
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}

			if server.ModelProxyURL != tt.want {
				t.Fatalf("URL = %q, want %q", server.ModelProxyURL, tt.want)
			}
		})
	}
}
