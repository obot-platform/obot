package events

import (
	"context"

	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
)

type Printer interface {
	Print(events <-chan types.Progress) error
}

func NewPrinter(ctx context.Context, c *apiclient.Client, quiet, details bool) Printer {
	if quiet {
		return &Quiet{
			Client: c,
			Ctx:    ctx,
		}
	}
	return &Verbose{
		Details: details,
		Client:  c,
		Ctx:     ctx,
	}
}
