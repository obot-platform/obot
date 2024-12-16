package cli

import (
	"fmt"

	"github.com/acorn-io/acorn/pkg/version"
	"github.com/spf13/cobra"
)

type Version struct{}

func (l *Version) Run(*cobra.Command, []string) error {
	fmt.Println("Version: ", version.Get())
	return nil
}
