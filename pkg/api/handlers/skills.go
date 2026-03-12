package handlers

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/controller/handlers/skillrepository"
	"github.com/obot-platform/obot/pkg/skillaccessrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	defaultSkillListLimit = 50
	maxSkillListLimit     = 200
)

type SkillHandler struct {
	skillAccessRuleHelper  *skillaccessrule.Helper
	materializeSkillSource func(ctx context.Context, skill *v1.Skill) (func(), string, error)
}

func NewSkillHandler(skillAccessRuleHelper *skillaccessrule.Helper) *SkillHandler {
	return &SkillHandler{
		skillAccessRuleHelper:  skillAccessRuleHelper,
		materializeSkillSource: skillrepository.MaterializeSkillSource,
	}
}

func (h *SkillHandler) List(req api.Context) error {
	limit, err := parseSkillListLimit(req.URL.Query().Get("limit"))
	if err != nil {
		return err
	}

	items, err := h.listAccessibleSkills(req, req.URL.Query().Get("repoID"))
	if err != nil {
		return err
	}

	query := strings.ToLower(strings.TrimSpace(req.URL.Query().Get("q")))
	filtered := make([]types.Skill, 0, len(items))
	for _, item := range items {
		if !item.Status.Valid {
			continue
		}
		if query != "" && !matchesSkillQuery(item, query) {
			continue
		}
		filtered = append(filtered, convertSkill(item))
	}

	slices.SortStableFunc(filtered, func(a, b types.Skill) int {
		aName := strings.ToLower(skillSortName(a.DisplayName, a.Name))
		bName := strings.ToLower(skillSortName(b.DisplayName, b.Name))
		if cmp := strings.Compare(aName, bName); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	return req.Write(types.SkillList{Items: filtered})
}

func (h *SkillHandler) Get(req api.Context) error {
	skill, err := h.getAccessibleSkill(req, req.PathValue("id"))
	if err != nil {
		return err
	}

	return req.Write(convertSkill(*skill))
}

func (h *SkillHandler) Download(req api.Context) error {
	skill, err := h.getAccessibleSkill(req, req.PathValue("id"))
	if err != nil {
		return err
	}

	cleanup, skillDir, err := h.materializeSkillSource(req.Context(), skill)
	if err != nil {
		return fmt.Errorf("failed to materialize skill source: %w", err)
	}
	defer cleanup()

	fileName := sanitizeDownloadFilename(skill.Spec.Name)
	if fileName == "" {
		fileName = skill.Name
	}

	req.ResponseWriter.Header().Set("Content-Type", "application/zip")
	req.ResponseWriter.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", fileName+".zip"))
	req.ResponseWriter.WriteHeader(http.StatusOK)

	return zipSkillDirectory(skillDir, req.ResponseWriter)
}

func (h *SkillHandler) getAccessibleSkill(req api.Context, id string) (*v1.Skill, error) {
	var skill v1.Skill
	if err := req.Get(&skill, id); err != nil {
		return nil, err
	}
	if !skill.Status.Valid {
		return nil, types.NewErrNotFound("skill %s not found", id)
	}

	hasAccess, err := h.skillAccessRuleHelper.UserHasAccessToSkill(req.User, &skill)
	if err != nil {
		return nil, err
	}
	if !hasAccess {
		return nil, types.NewErrNotFound("skill %s not found", id)
	}

	return &skill, nil
}

func (h *SkillHandler) listAccessibleSkills(req api.Context, repoID string) ([]v1.Skill, error) {
	allowAll, repoIDs, skillIDs, err := h.skillAccessRuleHelper.GetUserSkillAccessScope(req.User)
	if err != nil {
		return nil, err
	}

	if allowAll {
		return listSkills(req, repoID)
	}

	if repoID != "" {
		return h.listScopedSkillsForRepo(req, repoID, repoIDs, skillIDs)
	}

	results := make(map[string]v1.Skill, len(repoIDs)+len(skillIDs))
	for repoID := range repoIDs {
		skills, err := listSkills(req, repoID)
		if err != nil {
			return nil, err
		}
		for _, skill := range skills {
			results[skill.Name] = skill
		}
	}

	for skillID := range skillIDs {
		skill, err := getSkill(req, skillID)
		if err != nil {
			continue
		}
		results[skill.Name] = *skill
	}

	return mapValues(results), nil
}

func (h *SkillHandler) listScopedSkillsForRepo(req api.Context, repoID string, allowedRepoIDs, allowedSkillIDs map[string]struct{}) ([]v1.Skill, error) {
	if _, ok := allowedRepoIDs[repoID]; ok {
		return listSkills(req, repoID)
	}

	results := map[string]v1.Skill{}
	for skillID := range allowedSkillIDs {
		skill, err := getSkill(req, skillID)
		if err != nil || skill.Spec.RepoID != repoID {
			continue
		}
		results[skill.Name] = *skill
	}

	return mapValues(results), nil
}

func listSkills(req api.Context, repoID string) ([]v1.Skill, error) {
	var (
		list v1.SkillList
		opts []kclient.ListOption
	)

	if repoID != "" {
		opts = append(opts, kclient.MatchingFields{"spec.repoID": repoID})
	}
	if err := req.List(&list, opts...); err != nil {
		if repoID != "" {
			return nil, fmt.Errorf("failed to list skills for repository %s: %w", repoID, err)
		}
		return nil, fmt.Errorf("failed to list skills: %w", err)
	}

	return list.Items, nil
}

func getSkill(req api.Context, skillID string) (*v1.Skill, error) {
	var skill v1.Skill
	if err := req.Get(&skill, skillID); err != nil {
		return nil, err
	}
	return &skill, nil
}

func mapValues(values map[string]v1.Skill) []v1.Skill {
	return slices.Collect(maps.Values(values))
}

func parseSkillListLimit(raw string) (int, error) {
	if raw == "" {
		return defaultSkillListLimit, nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 {
		return 0, types.NewErrBadRequest("invalid limit: %s", raw)
	}
	if limit > maxSkillListLimit {
		return maxSkillListLimit, nil
	}
	return limit, nil
}

func matchesSkillQuery(skill v1.Skill, query string) bool {
	return strings.Contains(strings.ToLower(skill.Spec.Name), query) ||
		strings.Contains(strings.ToLower(skill.Spec.DisplayName), query) ||
		strings.Contains(strings.ToLower(skill.Spec.Description), query)
}

func skillSortName(displayName, name string) string {
	if displayName != "" {
		return displayName
	}
	return name
}

func zipSkillDirectory(skillDir string, w io.Writer) error {
	writer := zip.NewWriter(w)

	err := filepath.WalkDir(skillDir, func(currentPath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if currentPath == skillDir {
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			relPath, err := filepath.Rel(skillDir, currentPath)
			if err != nil {
				relPath = currentPath
			}
			return fmt.Errorf("symbolic links are not allowed in skill contents: %s", filepath.ToSlash(relPath))
		}
		if entry.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(skillDir, currentPath)
		if err != nil {
			return fmt.Errorf("failed to determine ZIP path for %s: %w", currentPath, err)
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("failed to stat %s: %w", currentPath, err)
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("failed to build ZIP header for %s: %w", currentPath, err)
		}
		header.Name = filepath.ToSlash(relPath)
		header.Method = zip.Deflate

		fileWriter, err := writer.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("failed to create ZIP entry for %s: %w", currentPath, err)
		}

		file, err := os.Open(currentPath)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", currentPath, err)
		}

		_, copyErr := io.Copy(fileWriter, file)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("failed to write ZIP entry for %s: %w", currentPath, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("failed to close %s: %w", currentPath, closeErr)
		}

		return nil
	})
	if err != nil {
		writer.Close()
		return err
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to finalize skill ZIP: %w", err)
	}

	return nil
}

func sanitizeDownloadFilename(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, ch := range strings.ToLower(value) {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
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

func convertSkill(skill v1.Skill) types.Skill {
	return types.Skill{
		Metadata:        MetadataFrom(&skill),
		SkillManifest:   skill.Spec.SkillManifest,
		RepoID:          skill.Spec.RepoID,
		RepoURL:         skill.Spec.RepoURL,
		RepoRef:         skill.Spec.RepoRef,
		CommitSHA:       skill.Spec.CommitSHA,
		RelativePath:    skill.Spec.RelativePath,
		InstallHash:     skill.Spec.InstallHash,
		Valid:           skill.Status.Valid,
		ValidationError: skill.Status.ValidationError,
		LastIndexedAt:   *types.NewTime(skill.Status.LastIndexedAt.Time),
	}
}
