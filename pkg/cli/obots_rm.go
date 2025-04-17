package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/cli/textio"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/spf13/cobra"
)

// DeleteObot implements the 'obot obot rm' subcommand
type DeleteObot struct {
	root  *Obot
	Force bool `usage:"Skip confirmation prompt" short:"f"`
}

func (c *DeleteObot) Customize(cmd *cobra.Command) {
	cmd.Use = "rm [ID...]"
	cmd.Short = "Delete one or more obots"
	cmd.Long = "Delete one or more obots by ID"
	cmd.Aliases = []string{"remove", "delete"}
	cmd.Args = cobra.MinimumNArgs(1)
}

func (c *DeleteObot) Run(cmd *cobra.Command, args []string) error {
	// Collect valid IDs first and validate them
	var validIDs []string
	var validProjects []*types.Project
	var errs []error

	for _, id := range args {
		// Check if ID has the project prefix (both p1- format and p1* format)
		if !strings.HasPrefix(id, system.ProjectPrefix) {
			errs = append(errs, fmt.Errorf("%s is not a valid obot ID (should start with p1)", id))
			continue
		}

		// Get project details to determine the assistant ID
		projectInfo, err := c.root.Client.GetProject(cmd.Context(), id)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to get obot details for %s: %w", id, err))
			continue
		}

		validIDs = append(validIDs, id)
		validProjects = append(validProjects, projectInfo)
	}

	if len(validIDs) == 0 {
		return errors.Join(errs...)
	}

	// If not forcing, confirm with user
	if !c.Force {
		fmt.Println("You are about to delete the following obots:")
		for i, project := range validProjects {
			fmt.Printf("  %s: %s\n", validIDs[i], project.Name)
		}

		response, err := textio.Ask("Confirm deletion [y/N]", "")
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}

		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Println("Deletion cancelled")
			return nil
		}
	}

	// Proceed with deletion
	for i, id := range validIDs {
		project := validProjects[i]
		if err := c.root.Client.DeleteProject(cmd.Context(), project.AssistantID, id); err != nil {
			errs = append(errs, fmt.Errorf("failed to delete obot %s: %w", id, err))
		} else {
			fmt.Printf("Obot deleted: %s (%s)\n", id, project.Name)
		}
	}

	return errors.Join(errs...)
}
