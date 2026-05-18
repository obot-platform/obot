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

	gptcmd "github.com/gptscript-ai/cmd"
	cliinternal "github.com/obot-platform/obot/pkg/cli/internal"
	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
	"github.com/obot-platform/obot/pkg/localagents"
	"github.com/spf13/cobra"
)

type Setup struct {
	URL            string `usage:"Obot app URL to configure"`
	Agents         string `usage:"Comma-separated target agents: detected, none, claude-code, or cursor" default:"detected"`
	Yes            bool   `usage:"Accept confirmations and use defaults"`
	NonInteractive bool   `usage:"Never read from stdin; fail if required input is missing"`
	Output         string `usage:"Output format: text or json" default:"text"`

	root *Obot
}

func (s *Setup) Customize(cmd *cobra.Command) {
	cmd.Use = "setup"
	cmd.Short = "Configure Obot locally and install supported agent bootstrap assets"
	cmd.Args = cobra.NoArgs
	cmd.AddCommand(gptcmd.Command(&SetupStatus{}))
	cmd.AddCommand(gptcmd.Command(&SetupDetectAgents{}))
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
	if s.NonInteractive {
		ctx = cliinternal.WithNonInteractive(ctx)
		cmd.SetContext(ctx)
	}
	if progress.json {
		ctx = cliinternal.WithOutputWriter(ctx, cmd.ErrOrStderr())
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
	if _, err := s.root.Client.GetToken(ctx, false, false); err != nil {
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
	fmt.Fprintf(cmd.ErrOrStderr(), "Logged in to %s\n", appURL)

	selection, err := parseSetupAgents(s.Agents)
	if err != nil {
		return err
	}
	if selection.none {
		if !progress.json {
			fmt.Fprintln(cmd.OutOrStdout(), "Skipping local agent bootstrap installation")
		}
		if err := progress.emit(setupProgressEvent{Type: setupProgressComplete, URL: appURL}); err != nil {
			return err
		}
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return setupErrorf(setupErrorAgentDetectionFailed, "failed to get user home dir: %w", err)
	}

	installers := localagents.DirectInstallers()
	detections := make(map[string]localagents.DetectionResult, len(installers))
	for _, installer := range installers {
		detections[installer.ID()] = installer.Detect(ctx)
	}

	if !progress.json {
		fmt.Fprintln(cmd.OutOrStdout(), "Detected:")
		for _, installer := range installers {
			detection := detections[installer.ID()]
			status := "missing"
			if detection.State == localagents.DetectionPresent {
				status = "installed"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %-15s %s\n", installer.DisplayName(), status)
		}
	}

	var installed []localagents.InstallResult
	var installErrs []error
	for _, installer := range installers {
		detection := detections[installer.ID()]
		shouldInstall := selection.detected && detection.State == localagents.DetectionPresent
		if selection.agentIDs[installer.ID()] {
			if detection.State != localagents.DetectionPresent {
				return setupErrorf(setupErrorAgentDetectionFailed, "%s was selected but was not detected", installer.DisplayName())
			}
			shouldInstall = true
		}
		if !shouldInstall {
			continue
		}
		result, err := installer.InstallBootstrap(ctx, home)
		if err != nil {
			installErrs = append(installErrs, setupErrorf(setupErrorAgentInstallFailed, "install bootstrap for %s: %w", installer.DisplayName(), err))
			continue
		}
		installed = append(installed, result)
		if err := progress.emit(setupProgressEvent{
			Type:        setupProgressAgentInstalled,
			AgentID:     result.AgentID,
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
			fmt.Fprintln(cmd.OutOrStdout(), "No supported local agents detected")
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
			return "", fmt.Errorf("configured Obot URL is %s; pass --yes to replace it with %s", cfg.DefaultURL, appURL)
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
	setupProgressAuthStarted    = "auth_started"
	setupProgressAuthCompleted  = "auth_completed"
	setupProgressConfigSaved    = "config_saved"
	setupProgressAgentInstalled = "agent_installed"
	setupProgressComplete       = "complete"
	setupProgressError          = "error"
)

type setupProgressEvent struct {
	Type        string   `json:"type"`
	Code        string   `json:"code,omitempty"`
	Message     string   `json:"message,omitempty"`
	URL         string   `json:"url,omitempty"`
	AgentID     string   `json:"agentID,omitempty"`
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
	setupErrorInvalidURL           setupErrorCode = "invalid_url"
	setupErrorServerUnreachable    setupErrorCode = "server_unreachable"
	setupErrorAuthUnavailable      setupErrorCode = "auth_unavailable"
	setupErrorAuthTimeout          setupErrorCode = "auth_timeout"
	setupErrorAuthCanceled         setupErrorCode = "auth_canceled"
	setupErrorConfigSaveFailed     setupErrorCode = "config_save_failed"
	setupErrorAgentDetectionFailed setupErrorCode = "agent_detection_failed"
	setupErrorAgentInstallFailed   setupErrorCode = "agent_install_failed"
	setupErrorUnknown              setupErrorCode = "unknown"
)

type setupCodedError struct {
	code setupErrorCode
	err  error
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

	return setupErrorUnknown
}

type setupAgentSelection struct {
	detected bool
	none     bool
	agentIDs map[string]bool
}

func parseSetupAgents(raw string) (setupAgentSelection, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = "detected"
	}

	selection := setupAgentSelection{
		agentIDs: map[string]bool{},
	}
	for part := range strings.SplitSeq(raw, ",") {
		value := strings.TrimSpace(part)
		switch value {
		case "":
			continue
		case "detected":
			selection.detected = true
		case "none":
			selection.none = true
		case localagents.ClaudeCodeAgentID:
			selection.agentIDs[localagents.ClaudeCodeAgentID] = true
		case localagents.CursorAgentID:
			selection.agentIDs[localagents.CursorAgentID] = true
		default:
			return setupAgentSelection{}, fmt.Errorf("unsupported --agents value %q; supported values are detected, none, claude-code, and cursor", value)
		}
	}

	if selection.none && (selection.detected || len(selection.agentIDs) > 0) {
		return setupAgentSelection{}, fmt.Errorf("--agents none cannot be combined with other values")
	}
	if !selection.none && !selection.detected && len(selection.agentIDs) == 0 {
		return setupAgentSelection{}, fmt.Errorf("--agents must include detected, none, claude-code, or cursor")
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
