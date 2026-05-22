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
	"setup.detectClients",
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

type SetupDetectClients struct {
	JSON bool `usage:"Print detected clients as JSON"`
}

func (s *SetupDetectClients) Customize(cmd *cobra.Command) {
	cmd.Use = "detect-clients"
	cmd.Short = "Detect supported local clients"
	cmd.Args = cobra.NoArgs
}

func (s *SetupDetectClients) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	result := detectSetupClients(ctx)
	if s.JSON {
		return writeJSON(cmd, result)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSTATE\tREASON")
	for _, client := range result.Clients {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			tableCell(client.ID),
			tableCell(client.DisplayName),
			tableCell(client.State),
			tableCell(client.Reason),
		)
	}
	return w.Flush()
}

func detectSetupClients(ctx context.Context) setupDetectClientsOutput {
	installers := localagents.DetectedAgents()
	result := setupDetectClientsOutput{
		Clients: make([]setupDetectedClient, 0, len(installers)),
	}
	for _, installer := range installers {
		detection := installer.Detect(ctx)
		result.Clients = append(result.Clients, setupDetectedClient{
			ID:          detection.AgentID,
			DisplayName: detection.DisplayName,
			State:       string(detection.State),
			Reason:      detection.Reason,
		})
	}
	return result
}

type setupDetectClientsOutput struct {
	Clients []setupDetectedClient `json:"clients"`
}

type setupDetectedClient struct {
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
