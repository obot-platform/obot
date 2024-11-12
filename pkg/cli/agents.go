package cli

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/otto8-ai/otto8/apiclient"
	"github.com/spf13/cobra"
)

type Agents struct {
	root  *Otto8
	Quiet bool `usage:"Only print IDs of agents" short:"q"`
	Wide  bool `usage:"Print more information" short:"w"`
}

func (l *Agents) Customize(cmd *cobra.Command) {
	cmd.Aliases = []string{"agent", "a"}
}

func (l *Agents) Run(cmd *cobra.Command, args []string) error {
	agents, err := l.root.Client.ListAgents(cmd.Context(), apiclient.ListAgentsOptions{})
	if err != nil {
		return err
	}

	if l.Quiet {
		for _, agent := range agents.Items {
			fmt.Println(agent.ID)
		}
		return nil
	}

	w := newTable("ID", "NAME", "DESCRIPTION", "INVOKE", "CREATED")
	for _, agent := range agents.Items {
		w.WriteRow(agent.ID, agent.Name, truncate(agent.Description, l.Wide), agent.Links["invoke"], humanize.Time(agent.Created.Time))
	}

	return w.Err()
}
