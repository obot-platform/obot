package skillrepository

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

const syncInterval = time.Hour

type repositoryFetcher interface {
	Fetch(ctx context.Context, repoURL, ref string) (*fetchedRepository, error)
	MaterializeCommit(ctx context.Context, repoURL, commitSHA string) (*fetchedRepository, error)
}

type Handler struct {
	fetcher repositoryFetcher
	now     func() time.Time
}

func New() *Handler {
	return &Handler{
		fetcher: newGitHubRepositoryFetcher(),
		now:     time.Now,
	}
}

func (h *Handler) Sync(req router.Request, resp router.Response) error {
	repo := req.Object.(*v1.SkillRepository)
	namespace := repo.Namespace

	forceSync := repo.Annotations[v1.SkillRepositorySyncAnnotation] == "true"
	if !forceSync && !repo.Status.LastSyncTime.IsZero() {
		timeSinceLastSync := h.now().Sub(repo.Status.LastSyncTime.Time)
		if timeSinceLastSync < syncInterval {
			resp.RetryAfter(syncInterval - timeSinceLastSync)
			return nil
		}
	}

	repo.Status.IsSyncing = true
	if err := req.Client.Status().Update(req.Ctx, repo); err != nil {
		return fmt.Errorf("failed to mark skill repository syncing: %w", err)
	}

	defer h.clearIsSyncing(req.Ctx, req.Client, namespace, repo.Name)

	fetched, err := h.fetcher.Fetch(req.Ctx, repo.Spec.RepoURL, repo.Spec.Ref)
	if err != nil {
		if statusErr := h.recordFailure(req.Ctx, req.Client, namespace, repo.Name, err); statusErr != nil {
			return statusErr
		}
		resp.RetryAfter(syncInterval)
		return nil
	}
	defer fetched.Cleanup()

	now := metav1.NewTime(h.now())
	skills, err := buildSkillsFromRepository(fetched.RepoRoot, repo, fetched.CommitSHA, now)
	if err != nil {
		if statusErr := h.recordFailure(req.Ctx, req.Client, namespace, repo.Name, err); statusErr != nil {
			return statusErr
		}
		resp.RetryAfter(syncInterval)
		return nil
	}

	if err := upsertSkills(req.Ctx, req.Client, repo.Namespace, repo.Name, skills); err != nil {
		if statusErr := h.recordFailure(req.Ctx, req.Client, namespace, repo.Name, err); statusErr != nil {
			return statusErr
		}
		return fmt.Errorf("failed to persist indexed skills: %w", err)
	}

	if err := h.recordSuccess(req.Ctx, req.Client, namespace, repo.Name, fetched.CommitSHA, len(skills)); err != nil {
		return err
	}

	if forceSync {
		if err := clearSyncAnnotation(req.Ctx, req.Client, namespace, repo.Name); err != nil {
			return err
		}
	}

	resp.RetryAfter(syncInterval)
	return nil
}

func upsertSkills(ctx context.Context, c client.Client, namespace, repoID string, skills []*v1.Skill) error {
	existingSkills, err := listSkillsForRepo(ctx, c, namespace, repoID)
	if err != nil {
		return err
	}

	desired := make(map[string]*v1.Skill, len(skills))
	for _, skill := range skills {
		desired[skill.Name] = skill
	}

	for _, skill := range skills {
		existing, ok := existingSkills[skill.Name]
		if !ok {
			if err := c.Create(ctx, skill); err != nil {
				return fmt.Errorf("failed to create skill %s: %w", skill.Name, err)
			}
			continue
		}

		existing.Spec = skill.Spec
		if err := c.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update skill %s: %w", skill.Name, err)
		}

		existing.Status = skill.Status
		if err := c.Status().Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update skill status %s: %w", skill.Name, err)
		}
	}

	for name, existing := range existingSkills {
		if _, ok := desired[name]; ok {
			continue
		}
		if err := c.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to prune skill %s: %w", name, err)
		}
	}

	return nil
}

func listSkillsForRepo(ctx context.Context, c client.Client, namespace, repoID string) (map[string]*v1.Skill, error) {
	var list v1.SkillList
	if err := c.List(ctx, &list, client.InNamespace(namespace), client.MatchingFields{"spec.repoID": repoID}); err != nil {
		return nil, fmt.Errorf("failed to list indexed skills: %w", err)
	}

	result := make(map[string]*v1.Skill, len(list.Items))
	for i := range list.Items {
		result[list.Items[i].Name] = &list.Items[i]
	}
	return result, nil
}

func (h *Handler) recordFailure(ctx context.Context, c client.Client, namespace, name string, syncErr error) error {
	var repo v1.SkillRepository
	if err := c.Get(ctx, router.Key(namespace, name), &repo); err != nil {
		return fmt.Errorf("failed to reload skill repository: %w", err)
	}

	repo.Status.LastSyncTime = metav1.NewTime(h.now())
	repo.Status.SyncError = syncErr.Error()
	return c.Status().Update(ctx, &repo)
}

func (h *Handler) recordSuccess(ctx context.Context, c client.Client, namespace, name, commitSHA string, skillCount int) error {
	var repo v1.SkillRepository
	if err := c.Get(ctx, router.Key(namespace, name), &repo); err != nil {
		return fmt.Errorf("failed to reload skill repository: %w", err)
	}

	repo.Status.LastSyncTime = metav1.NewTime(h.now())
	repo.Status.SyncError = ""
	repo.Status.ResolvedCommitSHA = commitSHA
	repo.Status.DiscoveredSkillCount = skillCount
	return c.Status().Update(ctx, &repo)
}

func (h *Handler) clearIsSyncing(ctx context.Context, c client.Client, namespace, name string) {
	var repo v1.SkillRepository
	if err := c.Get(ctx, router.Key(namespace, name), &repo); err != nil {
		if !apierrors.IsNotFound(err) {
			log.Errorf("failed to reload skill repository %s to clear syncing bit: %v", name, err)
		}
		return
	}

	if !repo.Status.IsSyncing {
		return
	}

	repo.Status.IsSyncing = false
	if err := c.Status().Update(ctx, &repo); err != nil && !apierrors.IsNotFound(err) {
		log.Errorf("failed to clear syncing bit for skill repository %s: %v", name, err)
	}
}

func clearSyncAnnotation(ctx context.Context, c client.Client, namespace, name string) error {
	var repo v1.SkillRepository
	if err := c.Get(ctx, router.Key(namespace, name), &repo); err != nil {
		return fmt.Errorf("failed to reload skill repository for annotation cleanup: %w", err)
	}

	if repo.Annotations == nil {
		return nil
	}
	if _, ok := repo.Annotations[v1.SkillRepositorySyncAnnotation]; !ok {
		return nil
	}

	delete(repo.Annotations, v1.SkillRepositorySyncAnnotation)
	return c.Update(ctx, &repo)
}

func materializeSkillSource(ctx context.Context, fetcher repositoryFetcher, skill *v1.Skill) (*fetchedRepository, string, error) {
	if skill.Spec.RepoURL == "" {
		return nil, "", fmt.Errorf("skill %s is missing repoURL", skill.Name)
	}
	if skill.Spec.CommitSHA == "" {
		return nil, "", fmt.Errorf("skill %s is missing commitSHA", skill.Name)
	}
	if skill.Spec.RelativePath == "" {
		return nil, "", fmt.Errorf("skill %s is missing relativePath", skill.Name)
	}

	fetched, err := fetcher.MaterializeCommit(ctx, skill.Spec.RepoURL, skill.Spec.CommitSHA)
	if err != nil {
		return nil, "", err
	}

	skillDir, err := safeJoinWithin(fetched.RepoRoot, skill.Spec.RelativePath)
	if err != nil {
		fetched.Cleanup()
		return nil, "", err
	}

	info, err := os.Lstat(skillDir)
	if err != nil {
		fetched.Cleanup()
		return nil, "", fmt.Errorf("failed to access skill path %q: %w", skill.Spec.RelativePath, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		fetched.Cleanup()
		return nil, "", fmt.Errorf("indexed skill path %q is a symbolic link", skill.Spec.RelativePath)
	}
	if !info.IsDir() {
		fetched.Cleanup()
		return nil, "", fmt.Errorf("indexed skill path %q is not a directory", skill.Spec.RelativePath)
	}

	return fetched, skillDir, nil
}

func MaterializeSkillSource(ctx context.Context, skill *v1.Skill) (func(), string, error) {
	fetched, skillDir, err := materializeSkillSource(ctx, newGitHubRepositoryFetcher(), skill)
	if err != nil {
		return nil, "", err
	}

	return fetched.Cleanup, skillDir, nil
}
