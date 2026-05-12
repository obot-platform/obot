package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	gptscript "github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	skillrepo "github.com/obot-platform/obot/pkg/controller/handlers/skillrepository"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SkillRepositoryHandler struct{}

func NewSkillRepositoryHandler() *SkillRepositoryHandler {
	return &SkillRepositoryHandler{}
}

func (*SkillRepositoryHandler) List(req api.Context) error {
	var list v1.SkillRepositoryList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list skill repositories: %w", err)
	}

	items := make([]types.SkillRepository, 0, len(list.Items))
	for _, item := range list.Items {
		tokenEnv, err := revealRepositoryTokens(req, item.Name)
		if err != nil {
			return err
		}
		items = append(items, convertSkillRepository(item, tokenEnv))
	}

	return req.Write(types.SkillRepositoryList{Items: items})
}

func (*SkillRepositoryHandler) Get(req api.Context) error {
	var repo v1.SkillRepository
	if err := req.Get(&repo, req.PathValue("skill_repository_id")); err != nil {
		return fmt.Errorf("failed to get skill repository: %w", err)
	}

	tokenEnv, err := revealRepositoryTokens(req, repo.Name)
	if err != nil {
		return err
	}
	return req.Write(convertSkillRepository(repo, tokenEnv))
}

func (*SkillRepositoryHandler) Create(req api.Context) error {
	manifest, err := readAndValidateSkillRepositoryManifest(req)
	if err != nil {
		return err
	}

	repo := v1.SkillRepository{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.SkillRepositoryPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.SkillRepositorySpec{
			SkillRepositoryManifest: *manifest,
		},
	}

	if err := req.Create(&repo); err != nil {
		return fmt.Errorf("failed to create skill repository: %w", err)
	}

	newTokens := mergeCatalogTokens([]string{manifest.RepoURL}, manifest.SourceURLCredentials, nil)
	if err := storeRepositoryTokens(req, repo.Name, newTokens, nil); err != nil {
		return err
	}

	return req.WriteCreated(convertSkillRepository(repo, newTokens))
}

func (*SkillRepositoryHandler) Update(req api.Context) error {
	manifest, err := readAndValidateSkillRepositoryManifest(req)
	if err != nil {
		return err
	}

	var repo v1.SkillRepository
	if err := req.Get(&repo, req.PathValue("skill_repository_id")); err != nil {
		return fmt.Errorf("failed to get skill repository: %w", err)
	}

	existingCred, err := revealRepositoryTokens(req, repo.Name)
	if err != nil {
		return err
	}

	newTokens := mergeCatalogTokens([]string{manifest.RepoURL}, manifest.SourceURLCredentials, existingCred)

	repo.Spec.SkillRepositoryManifest = *manifest
	if err := req.Update(&repo); err != nil {
		return fmt.Errorf("failed to update skill repository: %w", err)
	}

	if err := storeRepositoryTokens(req, repo.Name, newTokens, existingCred); err != nil {
		return err
	}

	return req.Write(convertSkillRepository(repo, newTokens))
}

func (*SkillRepositoryHandler) Delete(req api.Context) error {
	return req.Delete(&v1.SkillRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("skill_repository_id"),
			Namespace: req.Namespace(),
		},
	})
}

func (*SkillRepositoryHandler) Refresh(req api.Context) error {
	var repo v1.SkillRepository
	if err := req.Get(&repo, req.PathValue("skill_repository_id")); err != nil {
		return fmt.Errorf("failed to get skill repository: %w", err)
	}

	if repo.Annotations == nil {
		repo.Annotations = map[string]string{}
	}
	repo.Annotations[v1.SkillRepositorySyncAnnotation] = "true"

	if err := req.Update(&repo); err != nil {
		return fmt.Errorf("failed to refresh skill repository: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func readAndValidateSkillRepositoryManifest(req api.Context) (*types.SkillRepositoryManifest, error) {
	var manifest types.SkillRepositoryManifest
	if err := req.Read(&manifest); err != nil {
		return nil, types.NewErrBadRequest("failed to read skill repository manifest: %v", err)
	}

	untrimmedRef := manifest.Ref
	manifest.DisplayName = strings.TrimSpace(manifest.DisplayName)
	manifest.RepoURL = strings.TrimSpace(manifest.RepoURL)
	manifest.Ref = strings.TrimSpace(manifest.Ref)

	if manifest.DisplayName == "" {
		return nil, types.NewErrBadRequest("displayName is required")
	}
	if manifest.RepoURL == "" {
		return nil, types.NewErrBadRequest("repoURL is required")
	}
	if err := skillrepo.ValidateRepositoryURL(manifest.RepoURL); err != nil {
		return nil, types.NewErrBadRequest("invalid repoURL: %v", err)
	}
	if untrimmedRef != "" && manifest.Ref == "" {
		return nil, types.NewErrBadRequest("ref must not be empty when provided")
	}

	return &manifest, nil
}

func storeRepositoryTokens(req api.Context, repoName string, tokens, existing map[string]string) error {
	if len(tokens) > 0 {
		if err := req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
			Context:  repoName,
			ToolName: skillrepo.SkillRepositoryCredentialToolName,
			Type:     gptscript.CredentialTypeTool,
			Env:      tokens,
		}); err != nil {
			return fmt.Errorf("failed to store repository credentials: %w", err)
		}
	} else if len(existing) > 0 {
		if err := req.GPTClient.DeleteCredential(req.Context(), repoName, skillrepo.SkillRepositoryCredentialToolName); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
			return fmt.Errorf("failed to delete repository credentials: %w", err)
		}
	}
	return nil
}

func revealRepositoryTokens(req api.Context, repoName string) (map[string]string, error) {
	cred, err := req.GPTClient.RevealCredential(req.Context(), []string{repoName}, skillrepo.SkillRepositoryCredentialToolName)
	if err != nil {
		if errors.As(err, &gptscript.ErrNotFound{}) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to reveal credentials for repository %s: %w", repoName, err)
	}
	return cred.Env, nil
}

func convertSkillRepository(repo v1.SkillRepository, tokenEnv map[string]string) types.SkillRepository {
	manifest := repo.Spec.SkillRepositoryManifest
	manifest.SourceURLCredentials = maskCatalogCredentials([]string{repo.Spec.RepoURL}, tokenEnv)
	return types.SkillRepository{
		Metadata:                MetadataFrom(&repo),
		SkillRepositoryManifest: manifest,
		LastSyncTime:            *types.NewTime(repo.Status.LastSyncTime.Time),
		IsSyncing:               repo.Status.IsSyncing,
		SyncError:               repo.Status.SyncError,
		ResolvedCommitSHA:       repo.Status.ResolvedCommitSHA,
		DiscoveredSkillCount:    repo.Status.DiscoveredSkillCount,
	}
}
