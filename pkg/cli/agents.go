package cli

import (
	"fmt"

	"github.com/acorn-io/acorn/apiclient"
	"github.com/acorn-io/acorn/apiclient/types"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

type Agents struct {
	root   *Acorn
	Quiet  bool   `usage:"Only print IDs of agents" short:"q"`
	Wide   bool   `usage:"Print more information" short:"w"`
	Output string `usage:"Output format (table, json, yaml)" short:"o" default:"table"`
}

func (l *Agents) Customize(cmd *cobra.Command) {
	cmd.Aliases = []string{"agent", "a"}
}

func (l *Agents) Run(cmd *cobra.Command, args []string) error {
	var (
		agents types.AgentList
		err    error
	)

	if len(args) > 0 {
		for _, arg := range args {
			agent, err := l.root.Client.GetAgent(cmd.Context(), arg)
			if err != nil {
				return err
			}
			agents.Items = append(agents.Items, *agent)
		}
	} else {
		agents, err = l.root.Client.ListAgents(cmd.Context(), apiclient.ListAgentsOptions{})
		if err != nil {
			return err
		}
	}

	if ok, err := output(l.Output, agents); ok || err != nil {
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
