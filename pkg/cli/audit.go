package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/spf13/cobra"
)

const (
	auditClientClaudeCode = "claude-code"
	auditClientCodex      = "codex"
	auditClientCursor     = "cursor"
	auditClientVSCode     = "vscode"

	auditSourceLocalAgent = types.AuditLogSourceTypeLocalAgent
	auditEventToolCall    = types.AuditLogEventTypeToolCall

	auditPayloadRequestLimit  = 64 * 1024
	auditPayloadResponseLimit = 1024 * 1024
	auditPayloadErrorLimit    = 32 * 1024
	auditPayloadRawLimit      = 1024 * 1024

	auditSubmitTimeout = 2 * time.Second
	auditFlushTimeout  = 2 * time.Second
	auditFlushLimit    = 25
)

var auditLog = logger.Package()

type Audit struct {
	root *Obot
}

func (a *Audit) Customize(c *cobra.Command) {
	c.Use = "audit"
	c.Short = "Manage local audit event submission"
	c.Args = cobra.NoArgs
	c.AddCommand(cmd.Command(&AuditSubmit{root: a.root}))
	c.AddCommand(cmd.Command(&AuditFlush{root: a.root}))
	c.AddCommand(cmd.Command(&AuditStatus{}))
}

func (a *Audit) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

type AuditSubmit struct {
	Format string `usage:"Client hook payload format: claude-code, codex, cursor, or vscode"`
	Input  string `usage:"Input file path, or - for stdin" default:"-"`

	root *Obot
}

func (s *AuditSubmit) Customize(cmd *cobra.Command) {
	cmd.Use = "submit"
	cmd.Short = "Submit a local audit hook payload"
	cmd.Args = cobra.NoArgs
	cmd.Hidden = true
}

func (s *AuditSubmit) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	format := strings.TrimSpace(s.Format)
	if !supportedAuditClient(format) {
		return fmt.Errorf("unsupported --format value %q; supported values are claude-code, codex, cursor, and vscode", s.Format)
	}

	input, err := readAuditInput(cmd, s.Input)
	if err != nil {
		auditLog.Warnf("failed to read local audit hook input: %v", err)
		return nil
	}

	event, err := normalizeAuditEvent(ctx, format, input)
	if err != nil {
		auditLog.Warnf("failed to normalize local audit hook input: %v", err)
		return nil
	}

	if err := s.submitCurrent(ctx, event); err != nil {
		auditLog.Warnf("failed to submit local audit event %s: %v", event.EventID, err)
		if err := defaultAuditSpool().Write(event); err != nil {
			auditLog.Warnf("failed to spool local audit event %s; dropping event: %v", event.EventID, err)
		}
	}

	return nil
}

func (s *AuditSubmit) submitCurrent(ctx context.Context, event types.AuditEvent) error {
	client, err := auditAPIClient(ctx, s.root.Client, auditSubmitTimeout)
	if err != nil {
		return err
	}

	submitCtx, cancel := context.WithTimeout(ctx, auditSubmitTimeout)
	defer cancel()
	resp, err := client.SubmitAuditEvents(submitCtx, []types.AuditEvent{event})
	if err != nil {
		return err
	}
	if !auditSubmitAccepted(resp, event.EventID) {
		return fmt.Errorf("server rejected event %s", event.EventID)
	}

	flushCtx, flushCancel := context.WithTimeout(ctx, auditFlushTimeout)
	defer flushCancel()
	if _, err := flushAuditSpool(flushCtx, client, defaultAuditSpool(), auditFlushLimit); err != nil {
		auditLog.Warnf("failed to flush older local audit events: %v", err)
	}
	return nil
}

type AuditFlush struct {
	root *Obot
}

func (f *AuditFlush) Customize(cmd *cobra.Command) {
	cmd.Use = "flush"
	cmd.Short = "Submit pending local audit events"
	cmd.Args = cobra.NoArgs
}

func (f *AuditFlush) Run(cmd *cobra.Command, _ []string) error {
	client, err := auditAPIClient(cmd.Context(), f.root.Client, 15*time.Second)
	if err != nil {
		return err
	}
	flushed, err := flushAuditSpool(cmd.Context(), client, defaultAuditSpool(), 0)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Flushed %d audit events\n", flushed)
	return nil
}

func supportedAuditClient(format string) bool {
	return slices.Contains([]string{
		auditClientClaudeCode,
		auditClientCodex,
		auditClientCursor,
		auditClientVSCode,
	}, format)
}

func readAuditInput(cmd *cobra.Command, input string) ([]byte, error) {
	if strings.TrimSpace(input) == "" || input == "-" {
		return io.ReadAll(cmd.InOrStdin())
	}
	return os.ReadFile(input)
}
