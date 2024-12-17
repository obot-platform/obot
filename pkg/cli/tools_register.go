package cli

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
)

type ToolRegister struct {
	root  *Obot
	Quiet bool `usage:"Only print IDs of created tool references:" short:"q"`
}

func (l *ToolRegister) Customize(cmd *cobra.Command) {
	cmd.Use = "register [flags] NAME REFERENCE"
	cmd.Args = cobra.ExactArgs(2)
	cmd.Aliases = []string{"add", "create", "new"}
}

func (l *ToolRegister) Run(cmd *cobra.Command, args []string) error {
	tr, err := l.root.Client.CreateToolReference(cmd.Context(), types.ToolReferenceManifest{
		Name:      args[0],
		ToolType:  types.ToolReferenceTypeTool,
		Reference: args[1],
	})
	if err != nil {
		return err
	}
	if l.Quiet {
		fmt.Println(tr.ID)
	} else {
		fmt.Println("Tool reference created:", tr.ID)
	}
	return nil
}
