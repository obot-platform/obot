package cli

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
)

type AgentsKeys struct {
	root *Obot
}

type AgentsKeysList struct {
	PromptConfig

	ShowRevoked bool `usage:"Include revoked keys"`
	JSON        bool `usage:"Print results as JSON"`

	root *Obot
}

type AgentsKeysCreate struct {
	PromptConfig

	Name        string   `usage:"Name of the key"`
	Description string   `usage:"Description of the key"`
	ExpiresIn   string   `usage:"Expire the key after this duration, such as 720h" name:"expires-in"`
	ExpiresAt   string   `usage:"Expire the key at this RFC 3339 time" name:"expires-at"`
	MCP         []string `usage:"ID of a vMCP the key can connect to, or '*' for every vMCP the agent is granted; may be repeated" name:"mcp"`
	LLM         bool     `usage:"Allow the key to use the LLM proxy" name:"llm"`
	Skills      bool     `usage:"Allow the key to access skills"`
	JSON        bool     `usage:"Print the full response, including the key, as JSON"`

	root *Obot
}

type AgentsKeysGet struct {
	PromptConfig

	JSON bool `usage:"Print the key as JSON"`

	root *Obot
}

type AgentsKeysRevoke struct {
	PromptConfig

	root *Obot
}

func (k *AgentsKeys) Customize(c *cobra.Command) {
	c.Use = "keys"
	c.Short = "Manage an agent's API keys"
	c.Args = cobra.NoArgs
	c.AddCommand(
		cmd.Command(&AgentsKeysList{root: k.root}),
		cmd.Command(&AgentsKeysCreate{root: k.root}),
		cmd.Command(&AgentsKeysGet{root: k.root}),
		cmd.Command(&AgentsKeysRevoke{root: k.root}),
	)
}

func (k *AgentsKeys) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

func keyScopes(k types.AgentAPIKey) string {
	var scopes []string
	if len(k.MCPServerIDs) > 0 {
		scopes = append(scopes, "mcp:"+strings.Join(k.MCPServerIDs, ","))
	}
	if k.CanAccessLLMProxy {
		scopes = append(scopes, "llm")
	}
	if k.CanAccessSkills {
		scopes = append(scopes, "skills")
	}
	return dash(strings.Join(scopes, " "))
}

func keyState(k types.AgentAPIKey) string {
	switch {
	case k.RevokedAt != nil:
		return "revoked"
	case k.ExpiresAt != nil && k.ExpiresAt.Time.Before(time.Now()):
		return "expired"
	}
	return "active"
}

func writeKeyTable(w io.Writer, keys []types.AgentAPIKey) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tSTATE\tSCOPES\tEXPIRES\tLAST USED")
	for _, k := range keys {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", k.ID, tableCell(dash(k.Name)), keyState(k), tableCell(keyScopes(k)), formatAgentTime(k.ExpiresAt), formatAgentTime(k.LastUsedAt))
	}
	return tw.Flush()
}

func writeKey(w io.Writer, k types.AgentAPIKey) {
	fmt.Fprintf(w, "ID:           %d\n", k.ID)
	fmt.Fprintf(w, "Agent:        %s\n", k.AgentID)
	fmt.Fprintf(w, "Name:         %s\n", tableCell(dash(k.Name)))
	fmt.Fprintf(w, "Description:  %s\n", tableCell(dash(k.Description)))
	fmt.Fprintf(w, "State:        %s\n", keyState(k))
	fmt.Fprintf(w, "Scopes:       %s\n", keyScopes(k))
	fmt.Fprintf(w, "Created:      %s\n", formatAgentTime(&k.CreatedAt))
	fmt.Fprintf(w, "Expires:      %s\n", formatAgentTime(k.ExpiresAt))
	fmt.Fprintf(w, "Last used:    %s\n", formatAgentTime(k.LastUsedAt))
	if k.RevokedAt != nil {
		fmt.Fprintf(w, "Revoked:      %s\n", formatAgentTime(k.RevokedAt))
	}
}

func (k *AgentsKeysList) Customize(cmd *cobra.Command) {
	cmd.Use = "list <agent-id>"
	cmd.Short = "List an agent's API keys"
	cmd.Args = cobra.ExactArgs(1)
}

func (k *AgentsKeysList) Run(cmd *cobra.Command, args []string) error {
	client, err := agentsClient(cmd, k.root)
	if err != nil {
		return err
	}
	list, err := client.ListAgentAPIKeys(cmd.Context(), args[0], k.ShowRevoked)
	if err != nil {
		return mapAgentsError(cmd.Context(), client, err)
	}
	if k.JSON {
		return writeAgentsJSON(cmd.OutOrStdout(), list)
	}
	if len(list.Items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No API keys found")
		return nil
	}
	return writeKeyTable(cmd.OutOrStdout(), list.Items)
}

func (k *AgentsKeysCreate) Customize(cmd *cobra.Command) {
	cmd.Use = "create <agent-id>"
	cmd.Short = "Create an API key for an agent"
	cmd.Long = `Create an API key for an agent. A key needs at least one scope: --mcp, --llm or --skills.

The full key is shown only once. By default it is printed alone on stdout and the key's details go to stderr, so a script can capture it with KEY=$(obot agents keys create <agent-id> --llm). With --json, the whole response, including the key, is printed on stdout.`
	cmd.Example = `  obot agents keys create 7 --name ci --mcp '*' --expires-in 720h
  KEY=$(obot agents keys create 7 --llm --skills)`
	cmd.Args = cobra.ExactArgs(1)
}

func (k *AgentsKeysCreate) Run(cmd *cobra.Command, args []string) error {
	if k.ExpiresIn != "" && k.ExpiresAt != "" {
		return errors.New("--expires-in and --expires-at are mutually exclusive")
	}
	req := types.AgentAPIKeyCreateRequest{
		Name:              k.Name,
		Description:       k.Description,
		CanAccessLLMProxy: k.LLM,
		CanAccessSkills:   k.Skills,
		MCPServerIDs:      cleanList(k.MCP),
	}
	switch {
	case k.ExpiresIn != "":
		d, err := time.ParseDuration(k.ExpiresIn)
		if err != nil || d <= 0 {
			return fmt.Errorf("invalid --expires-in %q: use a positive duration such as 720h", k.ExpiresIn)
		}
		req.ExpiresAt = types.NewTime(time.Now().Add(d))
	case k.ExpiresAt != "":
		t, err := time.Parse(time.RFC3339, k.ExpiresAt)
		if err != nil {
			return fmt.Errorf("invalid --expires-at %q: use RFC 3339, such as 2030-01-02T15:04:05Z", k.ExpiresAt)
		}
		req.ExpiresAt = types.NewTime(t)
	}

	client, err := agentsClient(cmd, k.root)
	if err != nil {
		return err
	}
	resp, err := client.CreateAgentAPIKey(cmd.Context(), args[0], req)
	if err != nil {
		var httpErr *types.ErrHTTP
		if errors.As(err, &httpErr) && httpErr.Code == http.StatusBadRequest && len(req.MCPServerIDs) == 0 && !req.CanAccessLLMProxy && !req.CanAccessSkills {
			return fmt.Errorf("%w\nan agent key has no scope unless you ask for one: pass --mcp '*', --llm or --skills", err)
		}
		return mapAgentsError(cmd.Context(), client, err)
	}

	stderr := cmd.ErrOrStderr()
	fmt.Fprintln(stderr, "Save this key now: it will not be shown again.")
	if k.JSON {
		return writeAgentsJSON(cmd.OutOrStdout(), resp)
	}
	writeKey(stderr, resp.AgentAPIKey)
	fmt.Fprintln(cmd.OutOrStdout(), resp.Key)
	return nil
}

func (k *AgentsKeysGet) Customize(cmd *cobra.Command) {
	cmd.Use = "get <agent-id> <key-id>"
	cmd.Short = "Show an agent's API key"
	cmd.Args = cobra.ExactArgs(2)
}

func (k *AgentsKeysGet) Run(cmd *cobra.Command, args []string) error {
	client, err := agentsClient(cmd, k.root)
	if err != nil {
		return err
	}
	key, err := client.GetAgentAPIKey(cmd.Context(), args[0], args[1])
	if err != nil {
		return mapAgentsError(cmd.Context(), client, err)
	}
	if k.JSON {
		return writeAgentsJSON(cmd.OutOrStdout(), key)
	}
	writeKey(cmd.OutOrStdout(), *key)
	return nil
}

func (k *AgentsKeysRevoke) Customize(cmd *cobra.Command) {
	cmd.Use = "revoke <agent-id> <key-id>"
	cmd.Short = "Revoke an agent's API key"
	cmd.Args = cobra.ExactArgs(2)
}

func (k *AgentsKeysRevoke) Run(cmd *cobra.Command, args []string) error {
	client, err := agentsClient(cmd, k.root)
	if err != nil {
		return err
	}
	if err := client.RevokeAgentAPIKey(cmd.Context(), args[0], args[1]); err != nil {
		return mapAgentsError(cmd.Context(), client, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Revoked key %s of agent %s\n", args[1], args[0])
	return nil
}
