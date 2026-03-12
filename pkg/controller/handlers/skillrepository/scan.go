package skillrepository

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/skillformat"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildSkillsFromRepository(repoRoot string, repo *v1.SkillRepository, commitSHA string, indexedAt metav1.Time) ([]*v1.Skill, error) {
	directories, err := discoverSkillDirectories(repoRoot)
	if err != nil {
		return nil, err
	}

	result := make([]*v1.Skill, 0, len(directories))
	for _, dir := range directories {
		relPath, err := filepath.Rel(repoRoot, dir)
		if err != nil {
			return nil, fmt.Errorf("failed to determine repository-relative skill path: %w", err)
		}
		relPath = filepath.ToSlash(relPath)

		skill, err := buildSkill(dir, relPath, repo, commitSHA, indexedAt)
		if err != nil {
			return nil, err
		}
		result = append(result, skill)
	}

	return result, nil
}

func discoverSkillDirectories(repoRoot string) ([]string, error) {
	var directories []string

	err := filepath.WalkDir(repoRoot, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !entry.IsDir() {
			return nil
		}

		skillPath := filepath.Join(currentPath, skillformat.SkillMainFile)
		info, err := os.Lstat(skillPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("failed to inspect %s: %w", skillPath, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		directories = append(directories, currentPath)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(directories)
	return directories, nil
}

func buildSkill(dirPath, relPath string, repo *v1.SkillRepository, commitSHA string, indexedAt metav1.Time) (*v1.Skill, error) {
	content, err := os.ReadFile(filepath.Join(dirPath, skillformat.SkillMainFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read %s for %s: %w", skillformat.SkillMainFile, relPath, err)
	}
	if len(content) > maxSkillMDBytes {
		return nil, fmt.Errorf("%s for %s exceeds maximum size of %d bytes", skillformat.SkillMainFile, relPath, maxSkillMDBytes)
	}

	fm, _, parseErr := skillformat.ParseFrontmatter(string(content))
	skillName := fm.Name
	if skillName == "" {
		skillName = filepath.Base(dirPath)
	}

	skill := &v1.Skill{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.SchemeGroupVersion.String(),
			Kind:       "Skill",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      skillObjectName(repo.Name, relPath),
			Namespace: repo.Namespace,
		},
		Spec: v1.SkillSpec{
			SkillManifest: types.SkillManifest{
				Name:           skillName,
				Description:    fm.Description,
				DisplayName:    skillformat.DisplayName(skillName),
				License:        fm.License,
				Compatibility:  fm.Compatibility,
				AllowedTools:   fm.AllowedTools,
				MetadataValues: maps.Clone(fm.Metadata),
			},
			RepoID:       repo.Name,
			RepoURL:      repo.Spec.RepoURL,
			RepoRef:      repo.Spec.Ref,
			CommitSHA:    commitSHA,
			RelativePath: relPath,
		},
		Status: v1.SkillStatus{
			LastIndexedAt: indexedAt,
		},
	}

	validateErr := parseErr
	if validateErr == nil {
		validateErr = skillformat.ValidateFrontmatter(fm)
	}
	if validateErr == nil {
		validateErr = skillformat.ValidateNameMatchesDir(fm.Name, filepath.Base(dirPath))
	}
	if validateErr == nil {
		installHash, err := computeInstallHash(dirPath)
		if err != nil {
			validateErr = err
		} else {
			skill.Spec.InstallHash = installHash
		}
	}

	if validateErr != nil {
		skill.Status.Valid = false
		skill.Status.ValidationError = validateErr.Error()
		return skill, nil
	}

	skill.Status.Valid = true
	return skill, nil
}

func computeInstallHash(skillDir string) (string, error) {
	var files []string
	err := filepath.WalkDir(skillDir, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		files = append(files, currentPath)
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(files)
	digest := sha256.New()
	for _, filePath := range files {
		relPath, err := filepath.Rel(skillDir, filePath)
		if err != nil {
			return "", fmt.Errorf("failed to compute install hash path: %w", err)
		}
		relPath = filepath.ToSlash(relPath)
		if _, err := io.WriteString(digest, relPath); err != nil {
			return "", err
		}
		if _, err := digest.Write([]byte{0}); err != nil {
			return "", err
		}

		file, err := os.Open(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to open %s while computing install hash: %w", relPath, err)
		}
		if _, err := io.Copy(digest, file); err != nil {
			file.Close()
			return "", fmt.Errorf("failed to hash %s: %w", relPath, err)
		}
		if err := file.Close(); err != nil {
			return "", fmt.Errorf("failed to close %s while computing install hash: %w", relPath, err)
		}
		if _, err := digest.Write([]byte{0}); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(digest.Sum(nil)), nil
}

func skillObjectName(repoID, relPath string) string {
	fragment := sanitizeNameFragment(relPath)
	if fragment == "" {
		fragment = "skill"
	}
	return name.SafeHashConcatName(system.SkillPrefix, repoID, fragment)
}

func sanitizeNameFragment(value string) string {
	replacer := strings.NewReplacer("/", "-", "_", "-", ".", "-", " ", "-")
	value = strings.ToLower(replacer.Replace(value))

	var b strings.Builder
	lastDash := false
	for _, ch := range value {
		valid := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
		if valid {
			b.WriteRune(ch)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}

	return strings.Trim(b.String(), "-")
}
