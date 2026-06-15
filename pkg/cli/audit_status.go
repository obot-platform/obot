package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	cliinternal "github.com/obot-platform/obot/pkg/cli/internal"
	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
	"github.com/spf13/cobra"
)

type AuditStatus struct {
	JSON bool `usage:"Print status as JSON"`
}

func (s *AuditStatus) Customize(cmd *cobra.Command) {
	cmd.Use = "status"
	cmd.Short = "Show local audit submission status"
	cmd.Args = cobra.NoArgs
}

func (s *AuditStatus) Run(cmd *cobra.Command, _ []string) error {
	status, err := auditStatus(cmd.Context(), defaultAuditSpool())
	if err != nil {
		return err
	}
	if s.JSON {
		return writeJSON(cmd, status)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "Default URL:\t%s\n", tableCell(status.DefaultURL))
	fmt.Fprintf(w, "Token valid:\t%t\n", status.TokenValid)
	fmt.Fprintf(w, "Auth ready:\t%t\n", status.AuthReady)
	fmt.Fprintf(w, "Spool directory:\t%s\n", tableCell(status.SpoolDir))
	fmt.Fprintf(w, "Spool available:\t%t\n", status.SpoolAvailable)
	fmt.Fprintf(w, "Spool key available:\t%t\n", status.SpoolKeyAvailable)
	fmt.Fprintf(w, "Pending events:\t%d\n", status.PendingSpoolEvents)
	if status.SpoolError != "" {
		fmt.Fprintf(w, "Spool error:\t%s\n", tableCell(status.SpoolError))
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "CLIENT\tDETECTED\tINSTALLED\tMALFORMED\tCONFIG\tERROR")
	for _, hook := range status.Hooks {
		fmt.Fprintf(w, "%s\t%t\t%t\t%t\t%s\t%s\n",
			tableCell(hook.Client),
			hook.Detected,
			hook.Installed,
			hook.Malformed,
			tableCell(hook.ConfigPath),
			tableCell(hook.Error),
		)
	}
	return w.Flush()
}

type auditStatusOutput struct {
	DefaultURL         string            `json:"defaultURL"`
	TokenValid         bool              `json:"tokenValid"`
	AuthReady          bool              `json:"authReady"`
	SpoolDir           string            `json:"spoolDir"`
	SpoolAvailable     bool              `json:"spoolAvailable"`
	SpoolKeyAvailable  bool              `json:"spoolKeyAvailable"`
	PendingSpoolEvents int               `json:"pendingSpoolEvents"`
	SpoolError         string            `json:"spoolError,omitempty"`
	Hooks              []auditHookStatus `json:"hooks"`
}

type auditHookStatus struct {
	Client      string `json:"client"`
	DisplayName string `json:"displayName"`
	Detected    bool   `json:"detected"`
	Installed   bool   `json:"installed"`
	Malformed   bool   `json:"malformed"`
	ConfigPath  string `json:"configPath"`
	Error       string `json:"error,omitempty"`
}

func auditStatus(ctx context.Context, spool auditSpool) (auditStatusOutput, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cfg, err := localconfig.Load()
	if err != nil {
		return auditStatusOutput{}, fmt.Errorf("load Obot config: %w", err)
	}

	var tokenValid bool
	if cfg.DefaultURL != "" {
		checkCtx, cancel := context.WithTimeout(ctx, auditSubmitTimeout)
		tokenValid, err = cliinternal.StoredTokenValid(checkCtx, cfg.DefaultURL)
		cancel()
		if err != nil {
			return auditStatusOutput{}, fmt.Errorf("check stored token: %w", err)
		}
	}

	spoolDir, pending, keyAvailable, spoolErr := spool.Status()
	home, err := os.UserHomeDir()
	if err != nil {
		return auditStatusOutput{}, fmt.Errorf("get user home dir: %w", err)
	}
	detected := detectedAuditClients(ctx, home)
	hooks := make([]auditHookStatus, 0, len(auditSupportedClients()))
	for _, client := range auditSupportedClients() {
		hook := inspectAuditHook(home, client)
		hook.Detected = detected[client]
		hooks = append(hooks, hook)
	}

	out := auditStatusOutput{
		DefaultURL:         cfg.DefaultURL,
		TokenValid:         tokenValid,
		AuthReady:          cfg.DefaultURL != "" && tokenValid,
		SpoolDir:           spoolDir,
		SpoolAvailable:     spoolErr == nil,
		SpoolKeyAvailable:  keyAvailable,
		PendingSpoolEvents: pending,
		Hooks:              hooks,
	}
	if spoolErr != nil {
		out.SpoolError = spoolErr.Error()
	}
	return out, nil
}
