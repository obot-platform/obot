package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/localagents"
	"github.com/spf13/cobra"
)

type Skills struct {
	root *Obot
}

func (s *Skills) Customize(c *cobra.Command) {
	c.Use = "skills"
	c.Short = "Manage Obot skills"
	c.Args = cobra.NoArgs
	c.AddCommand(cmd.Command(&SkillsSearch{root: s.root}))
	c.AddCommand(cmd.Command(&SkillsInstall{root: s.root}))
}

func (s *Skills) Run(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

type SkillsSearch struct {
	Limit int  `usage:"Maximum number of skills to return" default:"50"`
	JSON  bool `usage:"Print results as JSON"`

	root *Obot
}

func (s *SkillsSearch) Customize(cmd *cobra.Command) {
	cmd.Use = "search [query]"
	cmd.Short = "Search Obot for installable skills"
	cmd.Args = cobra.MaximumNArgs(1)
}

func (s *SkillsSearch) Run(cmd *cobra.Command, args []string) error {
	if s.root == nil || s.root.Client == nil {
		return fmt.Errorf("skills search: no API client configured")
	}

	var query string
	if len(args) > 0 {
		query = strings.TrimSpace(args[0])
	}

	result, err := s.root.Client.ListSkills(cmd.Context(), query, s.Limit)
	if err != nil {
		return err
	}

	if s.JSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}

	if len(result.Items) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No skills found")
		return nil
	}

	return writeSkillSearchTable(cmd, result.Items)
}

func writeSkillSearchTable(cmd *cobra.Command, skills []types.Skill) error {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tDESCRIPTION\tREPOSITORY\tCOMPATIBILITY")
	for _, skill := range skills {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			tableCell(skill.ID),
			tableCell(skillDisplayName(skill)),
			tableCell(skill.Description),
			tableCell(skillRepository(skill)),
			tableCell(skill.Compatibility),
		)
	}
	return w.Flush()
}

func skillDisplayName(skill types.Skill) string {
	if skill.DisplayName != "" {
		return skill.DisplayName
	}
	return skill.Name
}

func skillRepository(skill types.Skill) string {
	if skill.RepoID != "" {
		return skill.RepoID
	}
	return skill.RepoURL
}

func tableCell(value string) string {
	return strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(value)
}

type SkillsInstall struct {
	Destination string `usage:"Target skills directory, such as ~/.claude/skills or ~/.agents/skills"`
	JSON        bool   `usage:"Print results as JSON"`

	root *Obot
}

func (s *SkillsInstall) Customize(cmd *cobra.Command) {
	cmd.Use = "install <skill-id>"
	cmd.Short = "Install an Obot skill into a local skills directory"
	cmd.Args = cobra.ExactArgs(1)
}

func (s *SkillsInstall) Run(cmd *cobra.Command, args []string) error {
	if s.root == nil || s.root.Client == nil {
		return fmt.Errorf("skills install: no API client configured")
	}

	destination, err := resolveSkillsDestination(strings.TrimSpace(s.Destination))
	if err != nil {
		return err
	}

	skillID := strings.TrimSpace(args[0])
	if skillID == "" {
		return fmt.Errorf("skill ID is required")
	}

	skill, err := s.root.Client.GetSkill(cmd.Context(), skillID)
	if err != nil {
		if isHTTPNotFound(err) {
			return fmt.Errorf("skill %q not found", skillID)
		}
		return err
	}
	if skill.ID == "" {
		return fmt.Errorf("skill %q has no ID", skillID)
	}

	data, err := s.root.Client.DownloadSkill(cmd.Context(), skill.ID)
	if err != nil {
		return err
	}
	archive, err := localagents.ParseSkillArchive(data, fallbackSkillName(skill))
	if err != nil {
		return err
	}

	output := skillsInstallOutput{
		Results: make([]skillsInstallResult, 0, 1),
	}
	name, installed, err := localagents.InstallSkillToRoot(cmd.Context(), destination, archive)
	if err != nil {
		return err
	}
	output.Results = append(output.Results, skillsInstallResult{
		Destination: destination,
		Mode:        "direct",
		Installed:   installed,
		Message:     fmt.Sprintf("Installed %s to %s", name, destination),
	})

	if s.JSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	}

	for _, result := range output.Results {
		fmt.Fprintln(cmd.OutOrStdout(), result.Message)
	}
	return nil
}

type skillsInstallOutput struct {
	Results []skillsInstallResult `json:"results"`
}

type skillsInstallResult struct {
	Destination string   `json:"destination"`
	Mode        string   `json:"mode"`
	Installed   []string `json:"installed,omitempty"`
	Message     string   `json:"message,omitempty"`
}

func resolveSkillsDestination(destination string) (string, error) {
	if destination == "" {
		return "", fmt.Errorf("--destination is required")
	}
	if destination == "~" || strings.HasPrefix(destination, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if home == "" {
			return "", fmt.Errorf("home directory is empty")
		}
		if destination == "~" {
			destination = home
		} else {
			destination = filepath.Join(home, strings.TrimPrefix(destination, "~/"))
		}
	}
	return filepath.Clean(destination), nil
}

func isHTTPNotFound(err error) bool {
	var httpErr *types.ErrHTTP
	return errors.As(err, &httpErr) && httpErr.Code == http.StatusNotFound
}

func fallbackSkillName(skill types.Skill) string {
	if skill.Name != "" {
		return skill.Name
	}
	if skill.DisplayName != "" {
		return skill.DisplayName
	}
	return skill.ID
}
