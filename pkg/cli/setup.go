package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	cliinternal "github.com/obot-platform/obot/pkg/cli/internal"
	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
	"github.com/obot-platform/obot/pkg/localagents"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type Setup struct {
	PromptConfig
	URL     string `usage:"Obot app URL to configure" local:"true"`
	Clients string `usage:"Comma-separated target clients: none, claude-code, or agents" local:"true"`
	Yes     bool   `usage:"Accept confirmations and use defaults" local:"true"`
	Output  string `usage:"Output format: text or json" default:"text" local:"true"`

	root *Obot
}

func (s *Setup) Customize(c *cobra.Command) {
	c.Use = "setup"
	c.Short = "Configure Obot locally and install supported client bootstrap assets"
	c.Args = cobra.NoArgs
	c.AddCommand(cmd.Command(&SetupStatus{}))
	c.AddCommand(cmd.Command(&SetupDetectClients{}))
}

func (s *Setup) Run(cmd *cobra.Command, _ []string) error {
	output := strings.TrimSpace(s.Output)
	if output == "" {
		output = "text"
	}
	if output != "text" && output != "json" {
		return fmt.Errorf("unsupported --output value %q; supported values are text and json", s.Output)
	}

	progress := newSetupProgressWriter(cmd, output == "json")
	if err := s.run(cmd, progress); err != nil {
		if progress.json {
			_ = progress.emit(setupProgressEvent{
				Type:    setupProgressError,
				Code:    string(setupErrorCodeFor(err)),
				Message: err.Error(),
			})
			return errorAlreadyReported{err: err}
		}
		return err
	}
	return nil
}

func (s *Setup) run(cmd *cobra.Command, progress setupProgressWriter) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if progress.json {
		if s.NonInteractive {
			ctx = cliinternal.WithOutputWriter(ctx, io.Discard)
		}
		cmd.SetContext(ctx)
		cmd.SetOut(cmd.ErrOrStderr())
	}

	appURL, err := s.resolveAppURL(cmd)
	if err != nil {
		return err
	}

	s.root.Client.BaseURL = localconfig.APIBaseURL(appURL)
	if err := progress.emit(setupProgressEvent{Type: setupProgressAuthStarted, URL: appURL}); err != nil {
		return err
	}
	if _, err := s.root.Client.GetToken(ctx, apiclient.TokenFetchOptions{
		Scopes: types.DefaultCLIAPIKeyScopes(),
	}); err != nil {
		return setupErrorf(setupAuthErrorCode(err), "authenticate with Obot: %w", err)
	}
	if err := progress.emit(setupProgressEvent{Type: setupProgressAuthCompleted, URL: appURL}); err != nil {
		return err
	}
	if err := localconfig.Save(localconfig.Config{DefaultURL: appURL}); err != nil {
		return setupErrorf(setupErrorConfigSaveFailed, "save Obot config: %w", err)
	}
	if err := progress.emit(setupProgressEvent{Type: setupProgressConfigSaved, URL: appURL}); err != nil {
		return err
	}
	if !progress.json {
		fmt.Fprintf(cmd.ErrOrStderr(), "Logged in to %s\n", appURL)
	}

	selection, err := s.resolveClientSelection(cmd)
	if err != nil {
		return err
	}
	if selection.none {
		if !progress.json {
			fmt.Fprintln(cmd.OutOrStdout(), "Skipping local client bootstrap installation")
		}
		if err := progress.emit(setupProgressEvent{Type: setupProgressComplete, URL: appURL}); err != nil {
			return err
		}
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return setupErrorf(setupErrorClientDetectionFailed, "failed to get user home dir: %w", err)
	}

	var installed []localagents.InstallResult
	var installErrs []error
	for _, target := range localagents.SetupTargets() {
		if !selection.clientIDs[target.ID()] {
			continue
		}
		result, err := target.InstallBootstrap(ctx, home)
		if err != nil {
			installErrs = append(installErrs, setupErrorf(setupErrorClientInstallFailed, "install bootstrap for %s: %w", target.DisplayName(), err))
			continue
		}
		installed = append(installed, result)
		if err := progress.emit(setupProgressEvent{
			Type:        setupProgressClientInstalled,
			ClientID:    result.AgentID,
			DisplayName: result.DisplayName,
			Installed:   result.Installed,
			Message:     result.Message,
		}); err != nil {
			return err
		}
	}
	if err := errors.Join(installErrs...); err != nil {
		return err
	}

	if len(installed) == 0 {
		if !progress.json {
			fmt.Fprintln(cmd.OutOrStdout(), "No local clients selected")
		}
		return progress.emit(setupProgressEvent{Type: setupProgressComplete, URL: appURL})
	}
	if !progress.json {
		for _, result := range installed {
			fmt.Fprintln(cmd.OutOrStdout(), result.Message)
		}
	}

	return progress.emit(setupProgressEvent{Type: setupProgressComplete, URL: appURL})
}

func (s *Setup) resolveClientSelection(cmd *cobra.Command) (setupClientSelection, error) {
	if cmd.Flags().Changed("clients") || strings.TrimSpace(s.Clients) != "" {
		return parseSetupClients(s.Clients)
	}
	if s.Yes {
		return parseSetupClients(localagents.SharedAgentsID)
	}
	if s.NonInteractive {
		return setupClientSelection{}, setupErrorf(setupErrorClientDetectionFailed, "--clients is required in non-interactive mode")
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	claudeDetected := false
	for _, agent := range localagents.DetectedAgents() {
		if agent.ID() == localagents.ClaudeCodeAgentID && agent.Detect(ctx).State == localagents.DetectionPresent {
			claudeDetected = true
			break
		}
	}

	raw, err := promptSetupClients(cmd, claudeDetected)
	if err != nil {
		return setupClientSelection{}, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "none"
	}
	selection, err := parseSetupClients(raw)
	if err != nil {
		return setupClientSelection{}, err
	}
	if selection.clientIDs[localagents.ClaudeCodeAgentID] && !claudeDetected {
		return setupClientSelection{}, setupErrorf(setupErrorClientDetectionFailed, "claude-code is only available from the interactive prompt when Claude Code is detected; pass --clients claude-code to install explicitly")
	}
	return selection, nil
}

func promptSetupClients(cmd *cobra.Command, claudeDetected bool) (string, error) {
	if setupPromptSupportsMenu(cmd) {
		return promptSetupClientsMenu(cmd, claudeDetected)
	}
	return promptSetupClientsLine(cmd, claudeDetected)
}

func promptSetupClientsMenu(cmd *cobra.Command, claudeDetected bool) (string, error) {
	options := []string{localagents.SharedAgentsID}
	if claudeDetected {
		options = append(options, localagents.ClaudeCodeAgentID)
	}

	selected := []string{localagents.SharedAgentsID}
	prompt := &survey.MultiSelect{
		Message:  "Install Obot bootstrap skills into:",
		Options:  options,
		Default:  selected,
		PageSize: len(options),
		Description: func(value string, _ int) string {
			switch value {
			case localagents.SharedAgentsID:
				return "All clients that support ~/.agents"
			case localagents.ClaudeCodeAgentID:
				return "Claude Code (detected)"
			default:
				return ""
			}
		},
	}

	in := cmd.InOrStdin().(interface {
		io.Reader
		Fd() uintptr
	})
	out := cmd.OutOrStdout().(interface {
		io.Writer
		Fd() uintptr
	})
	if err := survey.AskOne(prompt, &selected, survey.WithStdio(in, out, cmd.ErrOrStderr())); err != nil {
		return "", err
	}
	if len(selected) == 0 {
		return "none", nil
	}
	return strings.Join(selected, ","), nil
}

func promptSetupClientsLine(cmd *cobra.Command, claudeDetected bool) (string, error) {
	fmt.Fprintln(cmd.OutOrStdout(), "Choose local client skill targets:")
	fmt.Fprintln(cmd.OutOrStdout(), "  agents      All clients that support ~/.agents")
	if claudeDetected {
		fmt.Fprintln(cmd.OutOrStdout(), "  claude-code Claude Code (detected)")
	}
	return promptLine(cmd, "Clients to support (comma-separated; press Enter to skip): ")
}

func setupPromptSupportsMenu(cmd *cobra.Command) bool {
	in, ok := cmd.InOrStdin().(interface {
		io.Reader
		Fd() uintptr
	})
	if !ok || !term.IsTerminal(int(in.Fd())) {
		return false
	}
	out, ok := cmd.OutOrStdout().(interface {
		io.Writer
		Fd() uintptr
	})
	return ok && term.IsTerminal(int(out.Fd()))
}

func (s *Setup) resolveAppURL(cmd *cobra.Command) (string, error) {
	cfg, err := localconfig.Load()
	if err != nil {
		return "", fmt.Errorf("load Obot config: %w", err)
	}

	if strings.TrimSpace(s.URL) != "" {
		appURL, err := localconfig.NormalizeAppURL(s.URL)
		if err != nil {
			return "", setupErrorf(setupErrorInvalidURL, "%w", err)
		}
		if cfg.DefaultURL != "" && cfg.DefaultURL != appURL && !s.Yes {
			return "", setupErrorf(setupErrorInvalidURL, "configured Obot URL is %s; pass --yes to replace it with %s", cfg.DefaultURL, appURL)
		}
		return appURL, nil
	}

	if cfg.DefaultURL != "" {
		if s.Yes {
			return cfg.DefaultURL, nil
		}
		if s.NonInteractive {
			return "", setupErrorf(setupErrorInvalidURL, "configured Obot URL is %s; pass --yes to use it or pass --url to configure a different URL", cfg.DefaultURL)
		}

		ok, err := promptYesNo(cmd, fmt.Sprintf("Current Obot URL: %s\nUse this URL? [Y/n] ", cfg.DefaultURL), true)
		if err != nil {
			return "", err
		}
		if ok {
			return cfg.DefaultURL, nil
		}
	}

	if s.Yes {
		return "", setupErrorf(setupErrorInvalidURL, "--url is required when no configured Obot URL is accepted")
	}
	if s.NonInteractive {
		return "", setupErrorf(setupErrorInvalidURL, "--url is required in non-interactive mode when no configured Obot URL is accepted")
	}

	raw, err := promptLine(cmd, "Obot URL: ")
	if err != nil {
		return "", err
	}
	appURL, err := localconfig.NormalizeAppURL(raw)
	if err != nil {
		return "", setupErrorf(setupErrorInvalidURL, "%w", err)
	}
	return appURL, nil
}

const (
	setupProgressAuthStarted     = "auth_started"
	setupProgressAuthCompleted   = "auth_completed"
	setupProgressConfigSaved     = "config_saved"
	setupProgressClientInstalled = "client_installed"
	setupProgressComplete        = "complete"
	setupProgressError           = "error"
)

type setupProgressEvent struct {
	Type        string   `json:"type"`
	Code        string   `json:"code,omitempty"`
	Message     string   `json:"message,omitempty"`
	URL         string   `json:"url,omitempty"`
	ClientID    string   `json:"clientID,omitempty"`
	DisplayName string   `json:"displayName,omitempty"`
	Installed   []string `json:"installed,omitempty"`
}

type setupProgressWriter struct {
	json bool
	enc  *json.Encoder
}

func newSetupProgressWriter(cmd *cobra.Command, jsonOutput bool) setupProgressWriter {
	if !jsonOutput {
		return setupProgressWriter{}
	}
	return setupProgressWriter{
		json: true,
		enc:  json.NewEncoder(cmd.OutOrStdout()),
	}
}

func (w setupProgressWriter) emit(event setupProgressEvent) error {
	if !w.json {
		return nil
	}
	return w.enc.Encode(event)
}

type setupErrorCode string

const (
	setupErrorInvalidURL            setupErrorCode = "invalid_url"
	setupErrorServerUnreachable     setupErrorCode = "server_unreachable"
	setupErrorAuthUnavailable       setupErrorCode = "auth_unavailable"
	setupErrorAuthTimeout           setupErrorCode = "auth_timeout"
	setupErrorAuthCanceled          setupErrorCode = "auth_canceled"
	setupErrorConfigSaveFailed      setupErrorCode = "config_save_failed"
	setupErrorClientDetectionFailed setupErrorCode = "client_detection_failed"
	setupErrorClientInstallFailed   setupErrorCode = "client_install_failed"
	setupErrorAuthInvalid           setupErrorCode = "auth_invalid"
	setupErrorUnknown               setupErrorCode = "unknown"
)

type setupCodedError struct {
	code setupErrorCode
	err  error
}

type errorAlreadyReported struct {
	err error
}

func ErrorAlreadyReported(err error) bool {
	var reported errorAlreadyReported
	return errors.As(err, &reported)
}

func (e errorAlreadyReported) Error() string {
	return e.err.Error()
}

func (e errorAlreadyReported) Unwrap() error {
	return e.err
}

func setupErrorf(code setupErrorCode, format string, args ...any) error {
	return setupCodedError{
		code: code,
		err:  fmt.Errorf(format, args...),
	}
}

func (e setupCodedError) Error() string {
	return e.err.Error()
}

func (e setupCodedError) Unwrap() error {
	return e.err
}

func setupErrorCodeFor(err error) setupErrorCode {
	if coded, ok := errors.AsType[setupCodedError](err); ok {
		return coded.code
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return setupErrorAuthTimeout
	}
	if errors.Is(err, context.Canceled) {
		return setupErrorAuthCanceled
	}
	return setupErrorUnknown
}

func setupAuthErrorCode(err error) setupErrorCode {
	if errors.Is(err, context.DeadlineExceeded) {
		return setupErrorAuthTimeout
	}
	if errors.Is(err, context.Canceled) {
		return setupErrorAuthCanceled
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "no auth providers found"),
		strings.Contains(msg, "no configured auth providers found"),
		strings.Contains(msg, "multiple configured auth providers found"):
		return setupErrorAuthUnavailable
	}

	if _, ok := errors.AsType[*url.Error](err); ok {
		return setupErrorServerUnreachable
	}
	if strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "server misbehaving") {
		return setupErrorServerUnreachable
	}

	if strings.Contains(msg, "token does not have scope") {
		return setupErrorAuthInvalid
	}

	return setupErrorUnknown
}

type setupClientSelection struct {
	none      bool
	clientIDs map[string]bool
}

func parseSetupClients(raw string) (setupClientSelection, error) {
	raw = strings.TrimSpace(raw)

	selection := setupClientSelection{
		clientIDs: map[string]bool{},
	}
	for part := range strings.SplitSeq(raw, ",") {
		value := strings.TrimSpace(part)
		switch value {
		case "":
			continue
		case "none":
			selection.none = true
		case localagents.ClaudeCodeAgentID:
			selection.clientIDs[localagents.ClaudeCodeAgentID] = true
		case localagents.SharedAgentsID:
			selection.clientIDs[localagents.SharedAgentsID] = true
		default:
			return setupClientSelection{}, fmt.Errorf("unsupported --clients value %q; supported values are none, claude-code, and agents", value)
		}
	}

	if selection.none && len(selection.clientIDs) > 0 {
		return setupClientSelection{}, fmt.Errorf("--clients none cannot be combined with other values")
	}
	if !selection.none && len(selection.clientIDs) == 0 {
		return setupClientSelection{}, fmt.Errorf("--clients must include none, claude-code, or agents")
	}

	return selection, nil
}

func promptYesNo(cmd *cobra.Command, prompt string, defaultYes bool) (bool, error) {
	raw, err := promptLine(cmd, prompt)
	if err != nil {
		return false, err
	}
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return defaultYes, nil
	}
	switch raw {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("expected yes or no, got %q", raw)
	}
}

func promptLine(cmd *cobra.Command, prompt string) (string, error) {
	fmt.Fprint(cmd.OutOrStdout(), prompt)
	var b strings.Builder
	var one [1]byte
	for {
		n, err := cmd.InOrStdin().Read(one[:])
		if n > 0 {
			if one[0] == '\n' {
				return strings.TrimSpace(b.String()), nil
			}
			b.WriteByte(one[0])
		}
		if err != nil {
			if err == io.EOF && strings.TrimSpace(b.String()) != "" {
				return strings.TrimSpace(b.String()), nil
			}
			return "", err
		}
	}
}
