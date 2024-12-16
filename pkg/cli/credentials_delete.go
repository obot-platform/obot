package cli

import (
	"fmt"
	"strings"

	"github.com/acorn-io/acorn/apiclient"
	"github.com/acorn-io/acorn/pkg/system"
	"github.com/spf13/cobra"
)

type CredentialsDelete struct {
	root  *Acorn
	Quiet bool `usage:"Only print IDs of credentials" short:"q"`
}

func (l *CredentialsDelete) Customize(cmd *cobra.Command) {
	cmd.Use = "delete [flags] [CRED_NAME...]"
	cmd.Aliases = []string{"rm", "del", "d"}
}

func (l *CredentialsDelete) Run(cmd *cobra.Command, args []string) error {
	for _, arg := range args {
		opt := apiclient.DeleteCredentialsOptions{}
		scope, name, ok := strings.Cut(arg, "::")
		if !ok {
			name = scope
		} else if system.IsThreadID(scope) {
			opt.ThreadID = scope
		} else if system.IsAgentID(scope) {
			opt.AgentID = scope
		} else if system.IsWorkflowID(scope) {
			opt.WorkflowID = scope
		}
		if err := l.root.Client.DeleteCredential(cmd.Context(), name, opt); err != nil {
			return err
		}
		if l.Quiet {
			fmt.Println(name)
		} else {
			fmt.Printf("Credential %s deleted\n", name)
		}
	}
	return nil
}
