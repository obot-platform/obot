package cli

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

// Catalog implements the 'obot catalog' command
type Catalog struct {
	root   *Obot
	Quiet  bool   `usage:"Only print IDs of catalog obots" short:"q"`
	Wide   bool   `usage:"Print more information" short:"w"`
	Output string `usage:"Output format (table, json, yaml)" short:"o" default:"table"`
}

func (l *Catalog) Customize(cmd *cobra.Command) {
	cmd.Use = "catalog [flags]"
	cmd.Short = "Lists obots from the catalog"
	cmd.Long = "Lists all obots from the catalog (shared obots)"
	cmd.Aliases = []string{"cat", "c"}
}

func (l *Catalog) Run(cmd *cobra.Command, _ []string) error {
	shares, err := l.root.Client.ListProjectShares(cmd.Context())
	if err != nil {
		return err
	}

	if ok, err := output(l.Output, shares); ok || err != nil {
		return err
	}

	if l.Quiet {
		for _, share := range shares.Items {
			fmt.Println(share.PublicID)
		}
		return nil
	}

	w := newTable("ID", "NAME", "DESCRIPTION", "FEATURED", "PROJECT_ID", "CREATED")
	for _, share := range shares.Items {
		featured := "No"
		if share.Featured {
			featured = "Yes"
		}

		w.WriteRow(
			share.PublicID,
			share.Name,
			truncate(share.Description, l.Wide),
			featured,
			share.ProjectID,
			humanize.Time(share.Created.Time),
		)
	}

	return w.Err()
}
