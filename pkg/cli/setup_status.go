package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"text/tabwriter"

	cliinternal "github.com/obot-platform/obot/pkg/cli/internal"
	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
	"github.com/obot-platform/obot/pkg/localagents"
	"github.com/obot-platform/obot/pkg/version"
	"github.com/spf13/cobra"
)

var setupCapabilities = []string{
	"setup.nonInteractive",
	"setup.detectAgents",
	"setup.progressJSON",
}

type setupTokenValidator func(context.Context, string) (bool, error)

type SetupStatus struct {
	JSON bool `usage:"Print status as JSON"`

	tokenValid setupTokenValidator
}

func (s *SetupStatus) Customize(cmd *cobra.Command) {
	cmd.Use = "status"
	cmd.Short = "Show local Obot setup status"
	cmd.Args = cobra.NoArgs
}

func (s *SetupStatus) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	status, err := s.status(ctx)
	if err != nil {
		return err
	}

	if s.JSON {
		return writeJSON(cmd, status)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Version: %s\n", status.Version)
	fmt.Fprintf(cmd.OutOrStdout(), "Default URL: %s\n", status.DefaultURL)
	fmt.Fprintf(cmd.OutOrStdout(), "Token valid: %t\n", status.TokenValid)
	fmt.Fprintf(cmd.OutOrStdout(), "Setup complete: %t\n", status.SetupComplete)
	return nil
}

func (s *SetupStatus) status(ctx context.Context) (setupStatusOutput, error) {
	cfg, err := localconfig.Load()
	if err != nil {
		return setupStatusOutput{}, fmt.Errorf("load Obot config: %w", err)
	}

	var tokenValid bool
	if cfg.DefaultURL != "" {
		validator := s.tokenValid
		if validator == nil {
			validator = cliinternal.StoredTokenValid
		}
		tokenValid, err = validator(ctx, cfg.DefaultURL)
		if err != nil {
			return setupStatusOutput{}, fmt.Errorf("check stored token: %w", err)
		}
	}

	return setupStatusOutput{
		Version:       version.Get().String(),
		Capabilities:  slices.Clone(setupCapabilities),
		DefaultURL:    cfg.DefaultURL,
		TokenValid:    tokenValid,
		SetupComplete: cfg.DefaultURL != "" && tokenValid,
	}, nil
}

type setupStatusOutput struct {
	Version       string   `json:"version"`
	Capabilities  []string `json:"capabilities"`
	DefaultURL    string   `json:"defaultURL"`
	TokenValid    bool     `json:"tokenValid"`
	SetupComplete bool     `json:"setupComplete"`
}

type SetupDetectAgents struct {
	JSON bool `usage:"Print detected agents as JSON"`
}

func (s *SetupDetectAgents) Customize(cmd *cobra.Command) {
	cmd.Use = "detect-agents"
	cmd.Short = "Detect supported local agents"
	cmd.Args = cobra.NoArgs
}

func (s *SetupDetectAgents) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	result := detectSetupAgents(ctx)
	if s.JSON {
		return writeJSON(cmd, result)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATE\tREASON")
	for _, agent := range result.Agents {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			tableCell(agent.ID),
			tableCell(agent.DisplayName),
			tableCell(agent.State),
			tableCell(agent.Reason),
		)
	}
	return w.Flush()
}

func detectSetupAgents(ctx context.Context) setupDetectAgentsOutput {
	installers := localagents.DirectInstallers()
	result := setupDetectAgentsOutput{
		Agents: make([]setupDetectedAgent, 0, len(installers)),
	}
	for _, installer := range installers {
		detection := installer.Detect(ctx)
		result.Agents = append(result.Agents, setupDetectedAgent{
			ID:          detection.AgentID,
			DisplayName: detection.DisplayName,
			State:       string(detection.State),
			Reason:      detection.Reason,
		})
	}
	return result
}

type setupDetectAgentsOutput struct {
	Agents []setupDetectedAgent `json:"agents"`
}

type setupDetectedAgent struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	State       string `json:"state"`
	Reason      string `json:"reason"`
}

func writeJSON(cmd *cobra.Command, value any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
