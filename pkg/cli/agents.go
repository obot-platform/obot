package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type Agents struct {
	root *Obot
}

type AgentsList struct {
	PromptConfig

	IncludeDeleted bool `usage:"Include deleted agents"`
	JSON           bool `usage:"Print results as JSON"`

	root *Obot
}

type AgentsCreate struct {
	PromptConfig

	Name        string `usage:"Display name of the agent (required)"`
	Description string `usage:"Description of the agent"`
	JSON        bool   `usage:"Print the agent as JSON"`

	root *Obot
}

type AgentsGet struct {
	PromptConfig

	JSON bool `usage:"Print the agent as JSON"`

	root *Obot
}

type AgentsUpdate struct {
	PromptConfig

	Name        string `usage:"New display name"`
	Description string `usage:"New description; an empty value clears it"`
	JSON        bool   `usage:"Print the agent as JSON"`

	root *Obot
}

type AgentsDisable struct {
	PromptConfig

	Reason string `usage:"Why the agent is being disabled"`
	JSON   bool   `usage:"Print the agent as JSON"`

	root *Obot
}

type AgentsEnable struct {
	PromptConfig

	JSON bool `usage:"Print the agent as JSON"`

	root *Obot
}

type AgentsDelete struct {
	PromptConfig

	Yes bool `usage:"Delete without asking for confirmation"`

	root *Obot
}

func (a *Agents) Customize(c *cobra.Command) {
	c.Use = "agents"
	c.Short = "Manage agent identities and their API keys"
	c.Args = cobra.NoArgs
	c.AddCommand(
		cmd.Command(&AgentsList{root: a.root}),
		cmd.Command(&AgentsCreate{root: a.root}),
		cmd.Command(&AgentsGet{root: a.root}),
		cmd.Command(&AgentsUpdate{root: a.root}),
		cmd.Command(&AgentsDisable{root: a.root}),
		cmd.Command(&AgentsEnable{root: a.root}),
		cmd.Command(&AgentsDelete{root: a.root}),
		cmd.Command(&AgentsKeys{root: a.root}),
	)
}

func (a *Agents) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

func (a *AgentsList) Customize(cmd *cobra.Command) {
	cmd.Use = "list"
	cmd.Short = "List agent identities"
	cmd.Args = cobra.NoArgs
}

func (a *AgentsList) Run(cmd *cobra.Command, _ []string) error {
	client, err := agentsClient(cmd, a.root)
	if err != nil {
		return err
	}
	list, err := client.ListAgentIdentities(cmd.Context(), a.IncludeDeleted)
	if err != nil {
		return mapAgentsError(cmd.Context(), client, err)
	}
	if a.JSON {
		return writeAgentsJSON(cmd.OutOrStdout(), list)
	}
	if len(list.Items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No agents found")
		return nil
	}
	return writeAgentTable(cmd.OutOrStdout(), list.Items)
}

func (a *AgentsCreate) Customize(cmd *cobra.Command) {
	cmd.Use = "create --name <name>"
	cmd.Short = "Create an agent identity"
	cmd.Example = `  obot agents create --name "CI bot"`
	cmd.Args = cobra.NoArgs
}

func (a *AgentsCreate) Run(cmd *cobra.Command, _ []string) error {
	if strings.TrimSpace(a.Name) == "" {
		return errors.New("--name is required")
	}
	client, err := agentsClient(cmd, a.root)
	if err != nil {
		return err
	}
	agent, err := client.CreateAgentIdentity(cmd.Context(), types.AgentIdentityManifest{
		DisplayName: a.Name,
		Description: a.Description,
	})
	if err != nil {
		return mapAgentsError(cmd.Context(), client, err)
	}
	return printAgent(cmd, a.JSON, agent)
}

func (a *AgentsGet) Customize(cmd *cobra.Command) {
	cmd.Use = "get <agent-id>"
	cmd.Short = "Show an agent identity"
	cmd.Args = cobra.ExactArgs(1)
}

func (a *AgentsGet) Run(cmd *cobra.Command, args []string) error {
	client, err := agentsClient(cmd, a.root)
	if err != nil {
		return err
	}
	agent, err := client.GetAgentIdentity(cmd.Context(), args[0])
	if err != nil {
		return mapAgentsError(cmd.Context(), client, err)
	}
	return printAgent(cmd, a.JSON, agent)
}

func (a *AgentsUpdate) Customize(cmd *cobra.Command) {
	cmd.Use = "update <agent-id>"
	cmd.Short = "Update an agent identity"
	cmd.Long = "Update an agent identity. Only the fields whose flags are passed change; the others keep their current values."
	cmd.Args = cobra.ExactArgs(1)
}

func (a *AgentsUpdate) Run(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	nameSet, descSet := flags.Changed("name"), flags.Changed("description")
	if !nameSet && !descSet {
		return errors.New("nothing to update: pass --name or --description")
	}
	if nameSet && strings.TrimSpace(a.Name) == "" {
		return errors.New("--name cannot be empty")
	}

	client, err := agentsClient(cmd, a.root)
	if err != nil {
		return err
	}
	// The API replaces all metadata on PUT, so start from the current values and change only what was passed.
	current, err := client.GetAgentIdentity(cmd.Context(), args[0])
	if err != nil {
		return mapAgentsError(cmd.Context(), client, err)
	}
	manifest := current.AgentIdentityManifest
	if nameSet {
		manifest.DisplayName = a.Name
	}
	if descSet {
		manifest.Description = a.Description
	}

	updated, err := client.UpdateAgentIdentity(cmd.Context(), args[0], manifest)
	if err != nil {
		return mapAgentsError(cmd.Context(), client, err)
	}
	return printAgent(cmd, a.JSON, updated)
}

func (a *AgentsDisable) Customize(cmd *cobra.Command) {
	cmd.Use = "disable <agent-id>"
	cmd.Short = "Disable an agent identity"
	cmd.Args = cobra.ExactArgs(1)
}

func (a *AgentsDisable) Run(cmd *cobra.Command, args []string) error {
	client, err := agentsClient(cmd, a.root)
	if err != nil {
		return err
	}
	agent, err := client.DisableAgentIdentity(cmd.Context(), args[0], a.Reason)
	if err != nil {
		return mapAgentsError(cmd.Context(), client, err)
	}
	return printAgent(cmd, a.JSON, agent)
}

func (a *AgentsEnable) Customize(cmd *cobra.Command) {
	cmd.Use = "enable <agent-id>"
	cmd.Short = "Enable a disabled agent identity"
	cmd.Args = cobra.ExactArgs(1)
}

func (a *AgentsEnable) Run(cmd *cobra.Command, args []string) error {
	client, err := agentsClient(cmd, a.root)
	if err != nil {
		return err
	}
	agent, err := client.EnableAgentIdentity(cmd.Context(), args[0])
	if err != nil {
		return mapAgentsError(cmd.Context(), client, err)
	}
	return printAgent(cmd, a.JSON, agent)
}

func (a *AgentsDelete) Customize(cmd *cobra.Command) {
	cmd.Use = "delete <agent-id>"
	cmd.Short = "Delete an agent identity"
	cmd.Long = "Delete an agent identity. This revokes every one of its API keys and cannot be undone."
	cmd.Args = cobra.ExactArgs(1)
}

func (a *AgentsDelete) Run(cmd *cobra.Command, args []string) error {
	if !a.Yes {
		if !stdinIsTerminal(cmd) {
			return errors.New("refusing to delete without confirmation: pass --yes when stdin is not a terminal")
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Delete agent %s and revoke all of its API keys? This cannot be undone. [y/N]: ", args[0])
		line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
		if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
			return errors.New("aborted")
		}
	}
	client, err := agentsClient(cmd, a.root)
	if err != nil {
		return err
	}
	if err := client.DeleteAgentIdentity(cmd.Context(), args[0]); err != nil {
		return mapAgentsError(cmd.Context(), client, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Deleted agent %s\n", args[0])
	return nil
}

func stdinIsTerminal(cmd *cobra.Command) bool {
	in, ok := cmd.InOrStdin().(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(in.Fd()))
}

// agentsClient returns a client holding an API-scoped token. Every /api/agents route is admin-only and needs the API
// scope, which an admin's API-scoped user key carries along with their role.
func agentsClient(cmd *cobra.Command, root *Obot) (*apiclient.Client, error) {
	if root == nil || root.Client == nil {
		return nil, fmt.Errorf("agents: no API client configured")
	}
	client := root.Client
	// A fetched token replaces the stored login, so also request the default scopes rather than narrowing it. An
	// explicit token is only checked, so it only needs the API scope.
	scopes := []string{types.APIKeyScopeAPI}
	if client.Token == "" {
		scopes = append(types.DefaultCLIAPIKeyScopes(), types.APIKeyScopeAPI)
	}
	token, err := client.GetToken(cmd.Context(), apiclient.TokenFetchOptions{Scopes: scopes})
	if err != nil {
		return nil, fmt.Errorf("authenticate with Obot: %w", err)
	}
	return client.WithToken(token), nil
}

// mapAgentsError turns the HTTP errors that mean something specific for agent identities into clear messages, and
// returns every other error unchanged.
func mapAgentsError(ctx context.Context, client *apiclient.Client, err error) error {
	var httpErr *types.ErrHTTP
	if !errors.As(err, &httpErr) {
		return err
	}
	switch httpErr.Code {
	case http.StatusForbidden:
		return errors.New("this requires an administrator")
	case http.StatusNotFound:
		// A missing agent or key is also a 404. Listing agents only 404s on a server without the agent admin API, such
		// as an older version, so it tells the two apart.
		var probeErr *types.ErrHTTP
		if _, lerr := client.ListAgentIdentities(ctx, false); errors.As(lerr, &probeErr) && probeErr.Code == http.StatusNotFound {
			return errors.New("agent identities are not supported by this server")
		}
	}
	return err
}

func writeAgentsJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func formatAgentTime(t *types.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func writeAgentTable(w io.Writer, agents []types.AgentIdentity) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tACTIVE KEYS")
	for _, a := range agents {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", a.ID, tableCell(a.DisplayName), a.Status, a.ActiveAPIKeyCount)
	}
	return tw.Flush()
}

func writeAgent(w io.Writer, a *types.AgentIdentity) {
	fmt.Fprintf(w, "ID:           %s\n", a.ID)
	fmt.Fprintf(w, "Name:         %s\n", tableCell(a.DisplayName))
	fmt.Fprintf(w, "Description:  %s\n", tableCell(dash(a.Description)))
	fmt.Fprintf(w, "Status:       %s\n", a.Status)
	if a.Status == types.UserStatusDisabled {
		fmt.Fprintf(w, "Disabled at:  %s\n", formatAgentTime(a.DisabledAt))
		fmt.Fprintf(w, "Reason:       %s\n", tableCell(dash(a.DisabledReason)))
	}
	fmt.Fprintf(w, "Creator:      %s\n", dash(a.CreatorUserID))
	fmt.Fprintf(w, "Created:      %s\n", formatAgentTime(&a.Created))
	fmt.Fprintf(w, "Active keys:  %d\n", a.ActiveAPIKeyCount)
}

// printAgent prints a single agent as JSON or as key/value lines.
func printAgent(cmd *cobra.Command, asJSON bool, a *types.AgentIdentity) error {
	if asJSON {
		return writeAgentsJSON(cmd.OutOrStdout(), a)
	}
	writeAgent(cmd.OutOrStdout(), a)
	return nil
}

// cleanList drops blank entries, so that a flag passed as "" yields no values rather than one empty value.
func cleanList(values []string) []string {
	var out []string
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
