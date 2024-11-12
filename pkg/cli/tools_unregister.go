package cli

import (
	"fmt"

	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/spf13/cobra"
)

type ToolUnregister struct {
	root  *Otto8
	Quiet bool `usage:"Only print IDs of unregistered tool references" short:"q"`
}

func (l *ToolUnregister) Customize(cmd *cobra.Command) {
	cmd.Use = "unregister [flags] [ID...]"
	cmd.Aliases = []string{"rm", "del", "d"}
}

func (l *ToolUnregister) Run(cmd *cobra.Command, args []string) error {
	for _, arg := range args {
		if err := l.root.Client.DeleteToolReference(cmd.Context(), arg, types.ToolReferenceTypeTool); err != nil {
			return err
		}
		if l.Quiet {
			fmt.Println(arg)
		} else {
			fmt.Println("Tool reference deleted:", arg)
		}
	}
	return nil
}
