package cli

import (
	"fmt"

	"github.com/otto8-ai/otto8/pkg/version"
	"github.com/spf13/cobra"
)

type Version struct {
	root *Otto8
}

func (l *Version) Run(cmd *cobra.Command, args []string) error {
	fmt.Println("Version: ", version.Get())
	return nil
}
