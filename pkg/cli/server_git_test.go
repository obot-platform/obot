package cli

import (
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/spf13/cobra"
)

func TestGitMaxRepoSizeConfiguration(t *testing.T) {
	tests := []struct {
		name string
		env  string
		args []string
		want int
	}{
		{
			name: "default",
			want: 100,
		},
		{
			name: "environment",
			env:  "250",
			want: 250,
		},
		{
			name: "flag overrides environment",
			env:  "250",
			args: []string{"--git-max-repo-size-mb=500"},
			want: 500,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("OBOT_SERVER_GIT_MAX_REPO_SIZE_MB", tt.env)
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
			if server.GitMaxRepoSizeMB != tt.want {
				t.Fatalf("limit = %d, want %d", server.GitMaxRepoSizeMB, tt.want)
			}
		})
	}
}
