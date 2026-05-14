package localagents

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"path"
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

const (
	maxSkillArchiveFiles             = 50
	maxSkillArchiveEntryBytes        = 100 * 1024 * 1024
	maxSkillArchiveUncompressedBytes = 100 * 1024 * 1024
)

var (
	nonSkillNameChars = regexp.MustCompile(`[^a-z0-9-]+`)
	multipleDashes    = regexp.MustCompile(`-+`)
	windowsDrivePath  = regexp.MustCompile(`^[A-Za-z]:`)
)

// ParseSkillArchive validates downloaded skill ZIP bytes and returns a
// normalized archive rooted at the skill directory.
func ParseSkillArchive(data []byte, fallbackName string) (SkillArchive, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return SkillArchive{}, fmt.Errorf("invalid skill ZIP: %w", err)
	}
	if len(reader.File) > maxSkillArchiveFiles {
		return SkillArchive{}, fmt.Errorf("skill archive contains too many files (%d, max %d)", len(reader.File), maxSkillArchiveFiles)
	}

	entries := make([]SkillArchiveFile, 0, len(reader.File))
	var totalUncompressed uint64
	var totalRead int
	for _, file := range reader.File {
		if file.Mode().Type() == fs.ModeSymlink {
			return SkillArchive{}, fmt.Errorf("skill archive contains symlink %q", file.Name)
		}
		mode := file.Mode()
		if !mode.IsDir() && !mode.IsRegular() && mode.Type() != 0 {
			return SkillArchive{}, fmt.Errorf("skill archive contains unsupported file type %q", file.Name)
		}

		rel, err := cleanArchiveRelPath(file.Name)
		if err != nil {
			return SkillArchive{}, err
		}
		if file.FileInfo().IsDir() {
			continue
		}
		if file.UncompressedSize64 > uint64(maxSkillArchiveEntryBytes) {
			return SkillArchive{}, fmt.Errorf("skill archive entry %q exceeds maximum size of %d bytes", file.Name, maxSkillArchiveEntryBytes)
		}
		if totalUncompressed > uint64(maxSkillArchiveUncompressedBytes)-file.UncompressedSize64 {
			return SkillArchive{}, fmt.Errorf("skill archive total uncompressed size exceeds maximum size of %d bytes", maxSkillArchiveUncompressedBytes)
		}
		totalUncompressed += file.UncompressedSize64

		rc, err := file.Open()
		if err != nil {
			return SkillArchive{}, fmt.Errorf("failed to open archive entry %q: %w", file.Name, err)
		}
		remaining := maxSkillArchiveUncompressedBytes - totalRead
		readLimit := min(maxSkillArchiveEntryBytes, remaining)
		content, readErr := io.ReadAll(io.LimitReader(rc, int64(readLimit)+1))
		closeErr := rc.Close()
		if readErr != nil {
			return SkillArchive{}, fmt.Errorf("failed to read archive entry %q: %w", file.Name, readErr)
		}
		if len(content) > maxSkillArchiveEntryBytes {
			return SkillArchive{}, fmt.Errorf("skill archive entry %q exceeds maximum size of %d bytes", file.Name, maxSkillArchiveEntryBytes)
		}
		if len(content) > remaining {
			return SkillArchive{}, fmt.Errorf("skill archive total uncompressed size exceeds maximum size of %d bytes", maxSkillArchiveUncompressedBytes)
		}
		totalRead += len(content)
		if closeErr != nil {
			return SkillArchive{}, fmt.Errorf("failed to close archive entry %q: %w", file.Name, closeErr)
		}

		entries = append(entries, SkillArchiveFile{
			RelPath: rel,
			Content: content,
			Mode:    mode.Perm(),
		})
	}

	files, err := rootSkillFiles(entries)
	if err != nil {
		return SkillArchive{}, err
	}

	archive := SkillArchive{
		Name:  fallbackName,
		Files: files,
	}
	if name := archive.frontmatterName(); name != "" {
		archive.Name = name
	}
	if err := archive.validateFiles(); err != nil {
		return SkillArchive{}, err
	}
	if _, err := archive.installName(); err != nil {
		return SkillArchive{}, err
	}

	return archive, nil
}

func (s SkillArchive) ExtractTo(target string) error {
	if err := s.validateFiles(); err != nil {
		return err
	}

	files := make([]installFile, 0, len(s.Files))
	for _, file := range s.Files {
		rel, err := cleanArchiveRelPath(file.RelPath)
		if err != nil {
			return err
		}
		files = append(files, installFile{
			RelPath: rel,
			Content: file.Content,
			Mode:    file.Mode,
		})
	}

	return replaceDir(target, files)
}

func (s SkillArchive) installName() (string, error) {
	name := s.frontmatterName()
	if name == "" {
		name = sanitizeSkillName(s.Name)
	}
	if err := skillformat.ValidateName(name); err != nil {
		return "", fmt.Errorf("invalid skill name %q: %w", name, err)
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
	rel = strings.ReplaceAll(strings.TrimSpace(rel), "\\", "/")
	if rel == "" || rel == "." {
		return "", fmt.Errorf("skill archive contains empty file path")
	}
	if strings.HasPrefix(rel, "/") || windowsDrivePath.MatchString(rel) {
		return "", fmt.Errorf("skill archive contains absolute path %q", rel)
	}
	if strings.Contains(rel, ":") {
		return "", fmt.Errorf("skill archive contains non-portable path (colons not allowed) %q", rel)
	}
	if hasDotDotPathSegment(rel) {
		return "", fmt.Errorf("skill archive path escapes target directory: %q", rel)
	}

	cleaned := path.Clean(strings.TrimPrefix(rel, "./"))
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("skill archive contains empty file path")
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", fmt.Errorf("skill archive path escapes target directory: %q", rel)
	}

	return cleaned, nil
}

func hasDotDotPathSegment(rel string) bool {
	for segment := range strings.SplitSeq(rel, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

// rootSkillFiles returns archive files normalized so SKILL.md is at the
// install root. It accepts either root-level skill archives or archives with
// one extra top-level wrapper directory.
func rootSkillFiles(files []SkillArchiveFile) ([]SkillArchiveFile, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("skill archive contains no files")
	}

	// Preferred archive layout: SKILL.md is already at the archive root, so
	// all paths can be installed exactly as they appear.
	for _, file := range files {
		if file.RelPath == skillformat.SkillMainFile {
			return files, nil
		}
	}

	// Also accept archives created by zipping the skill directory itself:
	// <dir>/SKILL.md, <dir>/scripts/..., etc. That wrapper directory is not
	// part of the installed skill, so all files must share one top-level dir.
	var root string
	for _, file := range files {
		first, _, ok := strings.Cut(file.RelPath, "/")
		if !ok || first == "" {
			return nil, fmt.Errorf("skill archive missing %s", skillformat.SkillMainFile)
		}
		if root == "" {
			root = first
		} else if first != root {
			return nil, fmt.Errorf("skill archive has no root %s and contains multiple top-level directories", skillformat.SkillMainFile)
		}
	}

	// Strip the common wrapper directory and make the archive look like the
	// preferred root-level layout before handing it to installers.
	prefix := root + "/"
	rooted := make([]SkillArchiveFile, 0, len(files))
	for _, file := range files {
		rel := strings.TrimPrefix(file.RelPath, prefix)
		if rel == "" {
			continue
		}
		rooted = append(rooted, SkillArchiveFile{
			RelPath: rel,
			Content: file.Content,
			Mode:    file.Mode,
		})
	}

	for _, file := range rooted {
		if file.RelPath == skillformat.SkillMainFile {
			return rooted, nil
		}
	}
	return nil, fmt.Errorf("skill archive missing %s", skillformat.SkillMainFile)
}

func (s SkillArchive) frontmatterName() string {
	for _, file := range s.Files {
		rel, err := cleanArchiveRelPath(file.RelPath)
		if err != nil || rel != skillformat.SkillMainFile {
			continue
		}
		frontmatter, _, err := skillformat.ParseFrontmatter(string(file.Content))
		if err != nil {
			return ""
		}
		if err := skillformat.ValidateName(frontmatter.Name); err != nil {
			return ""
		}
		return frontmatter.Name
	}
	return ""
}

func sanitizeSkillName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = nonSkillNameChars.ReplaceAllString(name, "-")
	name = multipleDashes.ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}
