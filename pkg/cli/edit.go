package cli

import (
	"context"
	"fmt"

	"github.com/acorn-io/acorn/apiclient/types"
	"github.com/acorn-io/acorn/pkg/cli/edit"
	"github.com/acorn-io/acorn/pkg/system"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

type Edit struct {
	root   *Acorn
	Prompt bool `usage:"Edit just the prompt for the agent" short:"p"`
}

func (l *Edit) Customize(cmd *cobra.Command) {
	cmd.Args = cobra.ExactArgs(1)
}

func (l *Edit) Run(cmd *cobra.Command, args []string) error {
	id := args[0]
	if system.IsAgentID(id) {
		return l.editAgent(cmd.Context(), id)
	}
	return l.editWorkflow(cmd.Context(), id)
}

func (l *Edit) editWorkflow(ctx context.Context, id string) error {
	workflow, err := l.root.Client.GetWorkflow(ctx, id)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(workflow.WorkflowManifest)
	if err != nil {
		return err
	}

	err = edit.Edit(data, ".yaml", func(data []byte) error {
		var newManifest types.WorkflowManifest
		if err := yaml.Unmarshal(data, &newManifest); err != nil {
			return err
		}

		_, err := l.root.Client.UpdateWorkflow(ctx, workflow.ID, newManifest)
		return err
	})
	if err != nil {
		return err
	}
	fmt.Printf("Workflow updated: %s\n", workflow.ID)
	return nil
}

func (l *Edit) editAgent(ctx context.Context, id string) error {
	agent, err := l.root.Client.GetAgent(ctx, id)
	if err != nil {
		return err
	}

	if l.Prompt {
		err = edit.Edit([]byte(agent.Prompt), ".txt", func(data []byte) error {
			agent.Prompt = string(data)
			_, err := l.root.Client.UpdateAgent(ctx, agent.ID, agent.AgentManifest)
			return err
		})
		if err != nil {
			return err
		}
		fmt.Printf("Agent updated: %s\n", agent.ID)
		return nil
	}

	data, err := yaml.Marshal(agent.AgentManifest)
	if err != nil {
		return err
	}

	err = edit.Edit(data, ".yaml", func(data []byte) error {
		var newManifest types.AgentManifest
		if err := yaml.Unmarshal(data, &newManifest); err != nil {
			return err
		}

		_, err := l.root.Client.UpdateAgent(ctx, agent.ID, newManifest)
		return err
	})
	if err != nil {
		return err
	}
	fmt.Printf("Agent updated: %s\n", agent.ID)
	return nil
}
