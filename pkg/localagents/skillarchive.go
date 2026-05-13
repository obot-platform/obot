package localagents

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/obot-platform/obot/pkg/skillformat"
)

// SkillArchive is a validated skill payload ready for agent-specific
// installation. ZIP parsing and download-specific checks live outside
// DirectInstaller implementations.
type SkillArchive struct {
	Name  string
	Files []SkillArchiveFile
}

type SkillArchiveFile struct {
	RelPath string
	Content []byte
	Mode    fs.FileMode
}

var (
	nonSkillNameChars = regexp.MustCompile(`[^a-z0-9-]+`)
	multipleDashes    = regexp.MustCompile(`-+`)
)

func (s SkillArchive) installName() (string, error) {
	name := sanitizeSkillName(s.Name)
	if err := skillformat.ValidateName(name); err != nil {
		return "", fmt.Errorf("invalid skill name %q: %w", s.Name, err)
	}
	return name, nil
}

func (s SkillArchive) validateFiles() error {
	if len(s.Files) == 0 {
		return fmt.Errorf("skill archive contains no files")
	}

	seen := make(map[string]struct{}, len(s.Files))
	hasSkillMD := false
	for _, file := range s.Files {
		if file.Mode.Type() != 0 {
			return fmt.Errorf("skill archive contains non-regular file %q", file.RelPath)
		}
		rel, err := cleanArchiveRelPath(file.RelPath)
		if err != nil {
			return err
		}
		if rel == skillformat.SkillMainFile {
			hasSkillMD = true
		}
		if _, ok := seen[rel]; ok {
			return fmt.Errorf("skill archive contains duplicate file %q", rel)
		}
		seen[rel] = struct{}{}
	}

	if !hasSkillMD {
		return fmt.Errorf("skill archive missing %s", skillformat.SkillMainFile)
	}

	return nil
}

func cleanArchiveRelPath(rel string) (string, error) {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || rel == "." {
		return "", fmt.Errorf("skill archive contains empty file path")
	}
	if strings.HasPrefix(rel, "/") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("skill archive contains absolute path %q", rel)
	}

	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("skill archive contains empty file path")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("skill archive path escapes target directory: %q", rel)
	}

	return cleaned, nil
}

func sanitizeSkillName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = nonSkillNameChars.ReplaceAllString(name, "-")
	name = multipleDashes.ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}
