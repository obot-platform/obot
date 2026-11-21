package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
)

var (
	// toolListPollInterval is how often list-tools checks whether the user confirmed the request.
	toolListPollInterval = 2 * time.Second

	// openToolListURL opens the page where the user confirms a tool list request.
	openToolListURL = func(url string) error {
		browser.Stdout = os.Stderr
		return browser.OpenURL(url)
	}
)

type MCPListTools struct {
	PromptConfig

	NoBrowser bool `usage:"Print the confirmation URL instead of opening a browser"`
	JSON      bool `usage:"Print the tools as JSON"`

	root *Obot
}

type mcpListToolsOutput struct {
	EntryID   string             `json:"entryID"`
	EntryName string             `json:"entryName,omitempty"`
	Tools     []mcpListToolsTool `json:"tools"`
}

type mcpListToolsTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func (m *MCPListTools) Customize(cmd *cobra.Command) {
	cmd.Use = "list-tools <entry-id>"
	cmd.Short = "List a catalog entry's tools, after you confirm launching its server in Obot"
	cmd.Long = `List the tools of an MCP server catalog entry that has no stored tool list.

Listing launches the entry's server, so you confirm it in Obot: the command opens the request in your
browser and waits. It prints the tools on stdout, or as JSON with --json. Coding agents run it to get
an entry's real tool names; the confirmation stays with you.

If you aren't logged in to Obot, the command first logs you in, as obot login does. Agents pass
--non-interactive so that logging in never waits for input.

The entry id is the last part of the entry's URL in Obot, which the Obot Admin MCP server's tools return.`
	cmd.Example = `  obot mcp list-tools default-everything-29410f71
  obot mcp list-tools default-everything-29410f71 --non-interactive --json`
	cmd.Args = cobra.ExactArgs(1)
}

func (m *MCPListTools) Run(cmd *cobra.Command, args []string) error {
	if m.root == nil || m.root.Client == nil {
		return fmt.Errorf("mcp list-tools: no API client configured")
	}
	entryID := strings.TrimSpace(args[0])
	if entryID == "" {
		return fmt.Errorf("entry id is required")
	}

	// Logging in works as it does for obot login; --non-interactive keeps it from reading stdin.
	// Only the user can approve the login, like the confirmation that follows.
	client := m.root.Client
	if client.Token == "" {
		token, err := client.GetToken(cmd.Context(), apiclient.TokenFetchOptions{Scopes: types.DefaultCLIAPIKeyScopes()})
		if err != nil {
			return fmt.Errorf("log in to Obot: %w", err)
		}
		client = client.WithToken(token)
	}

	request, err := client.CreateToolListRequest(cmd.Context(), entryID)
	if err != nil {
		return fmt.Errorf("start listing the tools of %s: %w", entryID, err)
	}

	stderr := cmd.ErrOrStderr()
	name := request.EntryName
	if name == "" {
		name = entryID
	}
	if m.NoBrowser {
		fmt.Fprintf(stderr, "Listing the tools of %q launches its server. Confirm it in Obot:\n%s\n", name, request.URL)
	} else {
		fmt.Fprintf(stderr, "Listing the tools of %q launches its server. Confirm it in the Obot page opening in your browser.\n", name)
		if err := openToolListURL(request.URL); err != nil {
			fmt.Fprintf(stderr, "Open this URL in your browser:\n%s\n", request.URL)
		}
	}
	fmt.Fprintln(stderr, "Waiting for your confirmation...")

	done, err := waitForToolListRequest(cmd.Context(), stderr, client, request)
	if err != nil {
		return err
	}
	if done.Status != types.ToolListRequestCompleted {
		return fmt.Errorf("the tools of %q weren't listed: %s", name, done.Error)
	}

	output := mcpListToolsOutput{EntryID: done.EntryID, EntryName: done.EntryName, Tools: make([]mcpListToolsTool, 0, len(done.Tools))}
	for _, tool := range done.Tools {
		output.Tools = append(output.Tools, mcpListToolsTool{Name: tool.Name, Description: tool.Description})
	}
	if m.JSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}
	printListedTools(cmd.OutOrStdout(), name, output)
	return nil
}

// printListedTools prints the tools so that whoever reads them, a person or an agent, can use the
// names as they are.
func printListedTools(w io.Writer, name string, output mcpListToolsOutput) {
	if len(output.Tools) == 0 {
		fmt.Fprintf(w, "%s (%s) has no tools.\n", name, output.EntryID)
		return
	}
	fmt.Fprintf(w, "%s (%s) has %d tools. Use these exact names in allowedTools and toolOverrides:\n\n", name, output.EntryID, len(output.Tools))
	for _, tool := range output.Tools {
		if description := strings.Join(strings.Fields(tool.Description), " "); description != "" {
			fmt.Fprintf(w, "- %s: %s\n", tool.Name, description)
		} else {
			fmt.Fprintf(w, "- %s\n", tool.Name)
		}
	}
}

// waitForToolListRequest polls the request until the user answers it or it expires. Network errors
// and server errors don't end the wait, since the user may still confirm; an error Obot returns
// for the request itself, such as not found, does.
func waitForToolListRequest(ctx context.Context, stderr io.Writer, client *apiclient.Client, request types.ToolListRequest) (types.ToolListRequest, error) {
	if expires := request.ExpiresAt.GetTime(); !expires.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, expires)
		defer cancel()
	}
	ticker := time.NewTicker(toolListPollInterval)
	defer ticker.Stop()
	retrying := false
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				return request, fmt.Errorf("the request to list the tools of %q expired before it was confirmed", request.EntryID)
			}
			return request, ctx.Err()
		case <-ticker.C:
		}
		current, err := client.GetToolListRequest(ctx, request.ID)
		if err != nil {
			if ctx.Err() != nil {
				continue
			}
			if httpErr, ok := errors.AsType[*types.ErrHTTP](err); ok && httpErr.Code >= 400 && httpErr.Code < 500 {
				return request, fmt.Errorf("check the request to list the tools of %q: %w", request.EntryID, err)
			}
			if !retrying {
				fmt.Fprintf(stderr, "Couldn't reach Obot to check the request (%v); still waiting.\n", err)
				retrying = true
			}
			continue
		}
		retrying = false
		if current.Status != types.ToolListRequestPending {
			return current, nil
		}
	}
}
