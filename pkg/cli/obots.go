// filepath: /Users/wmaxwell/code/obot-platform/obot/pkg/cli/obots.go
package cli

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
)

type Obots struct {
	root   *Obot
	Quiet  bool   `usage:"Only print IDs of obots" short:"q"`
	Wide   bool   `usage:"Print more information" short:"w"`
	Output string `usage:"Output format (table, json, yaml)" short:"o" default:"table"`
	All    bool   `usage:"List all obots (admin only)" short:"a"`
}

func (l *Obots) Customize(cmd *cobra.Command) {
	cmd.Aliases = []string{"obot", "o"}
}

func (l *Obots) Run(cmd *cobra.Command, args []string) error {
	var (
		projects types.ProjectList
		err      error
	)

	if len(args) > 0 {
		for _, arg := range args {
			project, err := l.root.Client.GetProject(cmd.Context(), arg)
			if err != nil {
				return err
			}
			projects.Items = append(projects.Items, *project)
		}
	} else {
		projects, err = l.root.Client.ListProjects(cmd.Context(), apiclient.ListProjectsOptions{
			All: l.All,
		})
		if err != nil {
			return err
		}
	}

	if ok, err := output(l.Output, projects); ok || err != nil {
		return err
	}

	if l.Quiet {
		for _, project := range projects.Items {
			fmt.Println(project.ID)
		}
		return nil
	}

	w := newTable("ID", "NAME", "DESCRIPTION", "BASE AGENT", "CREATED")
	for _, project := range projects.Items {
		w.WriteRow(
			project.ID,
			project.Name,
			truncate(project.Description, l.Wide),
			project.AssistantID,
			humanize.Time(project.Created.Time),
		)
	}

	return w.Err()
}
