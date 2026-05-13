package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	gptcmd "github.com/gptscript-ai/cmd"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/spf13/cobra"
)

type Skills struct {
	root *Obot
}

func (s *Skills) Customize(cmd *cobra.Command) {
	cmd.Use = "skills"
	cmd.Short = "Manage Obot skills"
	cmd.Args = cobra.NoArgs
	cmd.AddCommand(gptcmd.Command(&SkillsSearch{root: s.root}))
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
