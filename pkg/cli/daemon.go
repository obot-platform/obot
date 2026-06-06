package cli

import (
	"context"
	"io"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
)

type Daemon struct{}

func (d *Daemon) Customize(cmd *cobra.Command) {
	cmd.Args = cobra.MinimumNArgs(1)
	cmd.Hidden = true
}

func (d *Daemon) Run(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	go func() {
		_, _ = io.ReadAll(os.Stdin)
		cancel()
	}()

	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Stderr = os.Stderr
	c.Stdout = os.Stdout
	c.Cancel = func() error {
		if runtime.GOOS == "windows" {
			return c.Process.Kill()
		}
		return c.Process.Signal(os.Interrupt)
	}
	return c.Run()
}
