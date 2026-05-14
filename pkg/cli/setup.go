package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/obot-platform/obot/pkg/cli/internal/localconfig"
	"github.com/obot-platform/obot/pkg/localagents"
	"github.com/spf13/cobra"
)

type Setup struct {
	URL    string `usage:"Obot app URL to configure"`
	Agents string `usage:"Comma-separated target agents: detected or claude-code" default:"detected"`
	Yes    bool   `usage:"Accept confirmations and use non-interactive defaults"`

	root *Obot
}

func (s *Setup) Customize(cmd *cobra.Command) {
	cmd.Use = "setup"
	cmd.Short = "Configure Obot locally and install supported agent bootstrap assets"
	cmd.Args = cobra.NoArgs
}

func (s *Setup) Run(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	appURL, err := s.resolveAppURL(cmd)
	if err != nil {
		return err
	}

	s.root.Client.BaseURL = localconfig.APIBaseURL(appURL)
	if _, err := s.root.Client.GetToken(ctx, false, false); err != nil {
		return err
	}
	if err := localconfig.Save(localconfig.Config{DefaultURL: appURL}); err != nil {
		return fmt.Errorf("save Obot config: %w", err)
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "Logged in to %s\n", appURL)

	selection, err := parseSetupAgents(s.Agents)
	if err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home dir: %w", err)
	}

	installers := localagents.DirectInstallers()
	detections := make(map[string]localagents.DetectionResult, len(installers))
	for _, installer := range installers {
		detections[installer.ID()] = installer.Detect(ctx)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Detected:")
	for _, installer := range installers {
		detection := detections[installer.ID()]
		status := "missing"
		if detection.State == localagents.DetectionPresent {
			status = "installed"
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %-15s %s\n", installer.DisplayName(), status)
	}

	var installed []localagents.InstallResult
	var installErrs []error
	for _, installer := range installers {
		detection := detections[installer.ID()]
		shouldInstall := selection.detected && detection.State == localagents.DetectionPresent
		if selection.agentIDs[installer.ID()] {
			if detection.State != localagents.DetectionPresent {
				return fmt.Errorf("%s was selected but was not detected", installer.DisplayName())
			}
			shouldInstall = true
		}
		if !shouldInstall {
			continue
		}
		result, err := installer.InstallBootstrap(ctx, home)
		if err != nil {
			installErrs = append(installErrs, fmt.Errorf("install bootstrap for %s: %w", installer.DisplayName(), err))
			continue
		}
		installed = append(installed, result)
	}
	if err := errors.Join(installErrs...); err != nil {
		return err
	}

	if len(installed) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No supported local agents detected")
		return nil
	}
	for _, result := range installed {
		fmt.Fprintln(cmd.OutOrStdout(), result.Message)
	}

	return nil
}

func (s *Setup) resolveAppURL(cmd *cobra.Command) (string, error) {
	cfg, err := localconfig.Load()
	if err != nil {
		return "", fmt.Errorf("load Obot config: %w", err)
	}

	if strings.TrimSpace(s.URL) != "" {
		appURL, err := localconfig.NormalizeAppURL(s.URL)
		if err != nil {
			return "", err
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

		ok, err := promptYesNo(cmd, fmt.Sprintf("Current Obot URL: %s\nUse this URL? [Y/n] ", cfg.DefaultURL), true)
		if err != nil {
			return "", err
		}
		if ok {
			return cfg.DefaultURL, nil
		}
	}

	if s.Yes {
		return "", fmt.Errorf("--url is required when no configured Obot URL is accepted")
	}

	raw, err := promptLine(cmd, "Obot URL: ")
	if err != nil {
		return "", err
	}
	return localconfig.NormalizeAppURL(raw)
}

type setupAgentSelection struct {
	detected bool
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
		case localagents.ClaudeCodeAgentID:
			selection.agentIDs[localagents.ClaudeCodeAgentID] = true
		default:
			return setupAgentSelection{}, fmt.Errorf("unsupported --agents value %q; supported values are detected and claude-code", value)
		}
	}

	if !selection.detected && len(selection.agentIDs) == 0 {
		return setupAgentSelection{}, fmt.Errorf("--agents must include detected or claude-code")
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
