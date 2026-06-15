package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type AuditSetup struct {
	Clients        string `usage:"Comma-separated clients to install: detected, all, claude-code, codex, cursor, or vscode"`
	Yes            bool   `usage:"Accept defaults without prompting; implies --clients detected when --clients is omitted"`
	NonInteractive bool   `usage:"Never read from stdin; fail if required input is missing"`
	JSON           bool   `usage:"Print setup result as JSON"`
}

func (s *AuditSetup) Customize(cmd *cobra.Command) {
	cmd.Use = "setup"
	cmd.Short = "Install local audit hooks for supported agent clients"
	cmd.Args = cobra.NoArgs
}

func (s *AuditSetup) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("get user home dir: %w", err)
	}
	binary, err := auditObotBinaryPath()
	if err != nil {
		return err
	}
	clients, err := s.resolveClients(cmd, ctx, home)
	if err != nil {
		return err
	}

	results := make([]auditHookSetupResult, 0, len(clients))
	for _, client := range clients {
		result := installAuditHook(ctx, home, binary, client)
		results = append(results, result)
	}
	output := auditHookSetupOutput{Results: results}
	if s.JSON {
		return writeJSON(cmd, output)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "CLIENT\tSTATUS\tCONFIG\tMESSAGE")
	for _, result := range results {
		status := "installed"
		if result.Malformed {
			status = "malformed"
		} else if result.Error != "" {
			status = "error"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			tableCell(result.Client),
			tableCell(status),
			tableCell(result.ConfigPath),
			tableCell(result.Message),
		)
	}
	return w.Flush()
}

func (s *AuditSetup) resolveClients(cmd *cobra.Command, ctx context.Context, home string) ([]string, error) {
	raw := strings.TrimSpace(s.Clients)
	switch {
	case raw != "":
	case s.Yes:
		raw = "detected"
	case s.NonInteractive || !auditSetupInteractive(cmd):
		return nil, fmt.Errorf("--clients is required in non-interactive mode unless --yes is provided")
	default:
		raw = "detected"
	}
	return parseAuditSetupClients(ctx, home, raw)
}

func auditSetupInteractive(cmd *cobra.Command) bool {
	in, ok := cmd.InOrStdin().(interface {
		Fd() uintptr
	})
	if !ok || !term.IsTerminal(int(in.Fd())) {
		return false
	}
	out, ok := cmd.OutOrStdout().(interface {
		Fd() uintptr
	})
	return ok && term.IsTerminal(int(out.Fd()))
}

func parseAuditSetupClients(ctx context.Context, home, raw string) ([]string, error) {
	selected := map[string]bool{}
	raw = strings.TrimSpace(raw)
	for part := range strings.SplitSeq(raw, ",") {
		value := strings.TrimSpace(part)
		switch value {
		case "":
			continue
		case "detected":
			if len(selected) > 0 {
				return nil, fmt.Errorf("--clients detected cannot be combined with other values")
			}
			detected := detectedAuditClients(ctx, home)
			out := make([]string, 0, len(detected))
			for _, client := range auditSupportedClients() {
				if detected[client] {
					out = append(out, client)
				}
			}
			return out, nil
		case "all":
			if len(selected) > 0 {
				return nil, fmt.Errorf("--clients all cannot be combined with other values")
			}
			return auditSupportedClients(), nil
		case auditClientClaudeCode, auditClientCodex, auditClientCursor, auditClientVSCode:
			selected[value] = true
		default:
			return nil, fmt.Errorf("unsupported --clients value %q; supported values are detected, all, claude-code, codex, cursor, and vscode", value)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("--clients must include detected, all, claude-code, codex, cursor, or vscode")
	}
	out := make([]string, 0, len(selected))
	for _, client := range auditSupportedClients() {
		if selected[client] {
			out = append(out, client)
		}
	}
	return out, nil
}

func auditSupportedClients() []string {
	return []string{auditClientClaudeCode, auditClientCodex, auditClientCursor, auditClientVSCode}
}

func detectedAuditClients(ctx context.Context, home string) map[string]bool {
	out := map[string]bool{}
	for _, client := range auditSupportedClients() {
		if ctx != nil && ctx.Err() != nil {
			return out
		}
		if auditClientDetected(home, client) {
			out[client] = true
		}
	}
	return out
}

func auditClientDetected(home, client string) bool {
	for _, bin := range auditClientBinaries(client) {
		if path, err := lookPath(bin); err == nil && path != "" {
			return true
		}
	}
	for _, rel := range auditClientDetectionPaths(client) {
		if _, err := os.Stat(filepath.Join(home, filepath.FromSlash(rel))); err == nil {
			return true
		}
	}
	return false
}

func auditClientBinaries(client string) []string {
	switch client {
	case auditClientClaudeCode:
		return []string{"claude"}
	case auditClientCodex:
		return []string{"codex"}
	case auditClientCursor:
		return []string{"cursor"}
	case auditClientVSCode:
		return []string{"code"}
	default:
		return nil
	}
}

func auditClientDetectionPaths(client string) []string {
	switch client {
	case auditClientClaudeCode:
		return []string{".claude", ".claude/settings.json"}
	case auditClientCodex:
		return []string{".codex", ".codex/hooks.json", ".codex/config.toml"}
	case auditClientCursor:
		return []string{".cursor", ".cursor/hooks.json"}
	case auditClientVSCode:
		return []string{".copilot/hooks", "Library/Application Support/Code", ".vscode"}
	default:
		return nil
	}
}

type auditHookSetupOutput struct {
	Results []auditHookSetupResult `json:"results"`
}

type auditHookSetupResult struct {
	Client     string `json:"client"`
	ConfigPath string `json:"configPath"`
	Installed  bool   `json:"installed"`
	Malformed  bool   `json:"malformed,omitempty"`
	Message    string `json:"message,omitempty"`
	Error      string `json:"error,omitempty"`
}
