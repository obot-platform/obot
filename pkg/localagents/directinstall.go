package localagents

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/obot-platform/obot/pkg/localagents/assets"
)

func resolveHome(explicit, configured string) (string, error) {
	if strings.TrimSpace(explicit) != "" {
		return explicit, nil
	}
	if strings.TrimSpace(configured) != "" {
		return configured, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	return home, nil
}

func installBootstrapAssets(skillsRoot string, rendered []assets.SkillAsset) ([]string, error) {
	bySkill := make(map[string][]installFile, len(rendered))
	for _, asset := range rendered {
		prefix := asset.SkillName + "/"
		if !strings.HasPrefix(asset.RelPath, prefix) {
			return nil, fmt.Errorf("bootstrap asset path %s is not under %s", asset.RelPath, asset.SkillName)
		}
		rel := strings.TrimPrefix(asset.RelPath, prefix)
		bySkill[asset.SkillName] = append(bySkill[asset.SkillName], installFile{
			RelPath: rel,
			Content: asset.Content,
			Mode:    0644,
		})
	}

	skillNames := make([]string, 0, len(bySkill))
	for skillName := range bySkill {
		skillNames = append(skillNames, skillName)
	}
	sort.Strings(skillNames)

	installed := make([]string, 0, len(rendered))
	for _, skillName := range skillNames {
		target := filepath.Join(skillsRoot, skillName)
		if err := replaceDir(target, bySkill[skillName]); err != nil {
			return nil, fmt.Errorf("install %s bootstrap skill: %w", skillName, err)
		}
		for _, file := range bySkill[skillName] {
			installed = append(installed, filepath.Join(target, filepath.FromSlash(file.RelPath)))
		}
	}
	sort.Strings(installed)
	return installed, nil
}

func installSkillArchiveToRoot(skillsRoot string, skill SkillArchive) (string, []string, error) {
	name, err := skill.installName()
	if err != nil {
		return "", nil, err
	}

	target := filepath.Join(skillsRoot, name)
	if err := skill.ExtractTo(target); err != nil {
		return "", nil, fmt.Errorf("install %s skill: %w", name, err)
	}

	installed := make([]string, 0, len(skill.Files))
	for _, file := range skill.Files {
		rel, err := cleanArchiveRelPath(file.RelPath)
		if err != nil {
			return "", nil, err
		}
		installed = append(installed, filepath.Join(target, filepath.FromSlash(rel)))
	}
	sort.Strings(installed)

	return name, installed, nil
}
