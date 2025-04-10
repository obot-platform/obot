package cli

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
)

// CreateObot implements the 'obot obot create' subcommand
type CreateObot struct {
	root      *Obot
	CatalogID string `usage:"ID of the base agent (catalog) to use for creating the obot" short:"c"`
	Name      string `usage:"Name for the new obot" short:"n"`
	ObotID    string `usage:"ID of an existing obot to copy directly" short:"i"`
}

func (c *CreateObot) Customize(cmd *cobra.Command) {
	cmd.Use = "create [flags]"
	cmd.Short = "Create a new obot"
	cmd.Long = "Create a new obot based on a specific agent, existing obot, or the default agent"
}

func (c *CreateObot) Run(cmd *cobra.Command, _ []string) error {
	var (
		assistantID string
		project     *types.Project
		err         error
	)

	// If obot-id is provided, directly copy the specified obot
	if c.ObotID != "" {
		// Try to get project details to determine the assistant ID
		projectInfo, err := c.root.Client.GetProject(cmd.Context(), c.ObotID)
		if err != nil {
			return fmt.Errorf("failed to get project details: %w", err)
		}

		assistantID = projectInfo.AssistantID

		// Directly copy the project
		project, err = c.root.Client.CopyProject(cmd.Context(), assistantID, c.ObotID)
		if err != nil {
			return fmt.Errorf("failed to copy obot %s: %w", c.ObotID, err)
		}

		// If a name was specified, update the project name
		if c.Name != "" {
			project.Name = c.Name
			project, err = c.root.Client.UpdateProject(cmd.Context(), project)
			if err != nil {
				return fmt.Errorf("failed to update project name: %w", err)
			}
		}
	} else if c.CatalogID != "" {
		// Step 1: Create a project from the share (catalog)
		// POST /api/shares/{share_public_id}
		project, err = c.root.Client.CreateProjectFromShare(cmd.Context(), c.CatalogID, true)
		if err != nil {
			return fmt.Errorf("failed to create project from catalog: %w", err)
		}

		// Step 2: Copy the project
		// POST /api/assistants/{assistant_id}/projects/{project_id}/copy
		project, err = c.root.Client.CopyProject(cmd.Context(), project.AssistantID, project.ID)
		if err != nil {
			return fmt.Errorf("failed to copy project from catalog: %w", err)
		}

		// If a name was specified, update the project name
		if c.Name != "" {
			project.Name = c.Name
			project, err = c.root.Client.UpdateProject(cmd.Context(), project)
			if err != nil {
				return fmt.Errorf("failed to update project name: %w", err)
			}
		}
	} else {
		// Get list of agents to find the default one
		agents, err := c.root.Client.ListAgents(cmd.Context(), apiclient.ListAgentsOptions{})
		if err != nil {
			return fmt.Errorf("failed to list agents: %w", err)
		}

		// Find the default agent
		for _, agent := range agents.Items {
			if agent.Default {
				assistantID = agent.ID
				break
			}
		}

		if assistantID == "" {
			// If no default is marked, use the first agent as a fallback
			if len(agents.Items) > 0 {
				assistantID = agents.Items[0].ID
			} else {
				return fmt.Errorf("no agents found to use as base")
			}
		}

		// Create the manifest for the new project
		manifest := types.ProjectManifest{
			ThreadManifest: types.ThreadManifest{
				ThreadManifestManagedFields: types.ThreadManifestManagedFields{
					Name:        c.Name,
					Description: "",
				},
			},
		}

		// Create the project
		project, err = c.root.Client.CreateProject(cmd.Context(), assistantID, manifest)
		if err != nil {
			return fmt.Errorf("failed to create obot: %w", err)
		}
	}

	// Print the result
	fmt.Printf("Created new obot: %s (ID: %s) using base agent: %s\n", project.Name, project.ID, project.AssistantID)
	return nil
}
