package data

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentcatalog"
	"github.com/obot-platform/obot/pkg/controller/handlers/skillrepository"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

//go:embed default-model-aliases.yaml
var defaultModelAliasesData []byte

//go:embed everything-access-control-rule.yaml
var everythingAccessControlRuleData []byte

//go:embed everything-skill-access-rule.yaml
var everythingSkillAccessRuleData []byte

//go:embed everything-hosted-agent-access-rule.yaml
var everythingHostedAgentAccessRuleData []byte

// Defaults are the seed values for the resources that point at Obot's own
// content repositories. They are grouped rather than passed positionally
// because they are all strings, and a swapped pair would only surface as a
// repository that fails to sync at runtime.
type Defaults struct {
	SkillRepoURL           string
	SkillRepoRef           string
	HostedAgentsCatalogURL string
	HostedAgentsCatalogRef string
	AllowLocalRepos        bool
}

func Data(ctx context.Context, c kclient.Client, defaults Defaults) error {
	var defaultModelAliases v1.DefaultModelAliasList
	if err := yaml.Unmarshal(defaultModelAliasesData, &defaultModelAliases); err != nil {
		return fmt.Errorf("failed to unmarshal default model aliases: %w", err)
	}

	defaultModelAccessPolicyResources := make([]types.ModelResource, 0, len(defaultModelAliases.Items))
	for _, alias := range defaultModelAliases.Items {
		var existing v1.DefaultModelAlias
		if err := c.Get(ctx, kclient.ObjectKey{Namespace: alias.Namespace, Name: alias.Name}, &existing); apierrors.IsNotFound(err) {
			if err := c.Create(ctx, &alias); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		if !modelaccesspolicy.IsAllowedDefaultModelAlias(alias.Name) {
			continue
		}

		// Build the default model access policy dynamically from default model aliases
		defaultModelAccessPolicyResources = append(defaultModelAccessPolicyResources, types.ModelResource{
			ID: types.DefaultModelAliasRefPrefix + alias.Name,
		})
	}

	var policies v1.ModelAccessPolicyList
	// Only create the "default models" model access policy if there are no existing policies
	if err := c.List(ctx, &policies); err != nil {
		return err
	} else if len(policies.Items) == 0 && len(defaultModelAccessPolicyResources) > 0 {
		if err := kclient.IgnoreAlreadyExists(c.Create(ctx, &v1.ModelAccessPolicy{
			Name:      system.ModelAccessPolicyPrefix + "-default",
			Namespace: system.DefaultNamespace,
			Spec: v1.ModelAccessPolicySpec{
				Manifest: types.ModelAccessPolicyManifest{
					DisplayName: "Default Policy",
					Subjects: []types.Subject{{
						Type: types.SubjectTypeSelector,
						ID:   "*",
					}},
					Models: defaultModelAccessPolicyResources,
				},
			},
		})); err != nil {
			return err
		}
	}

	var everythingAccessControlRule v1.AccessControlRule
	if err := yaml.Unmarshal(everythingAccessControlRuleData, &everythingAccessControlRule); err != nil {
		return fmt.Errorf("failed to unmarshal everything access control rule: %w", err)
	}

	var catalogs v1.MCPCatalogList
	// Only seed default access/skill rules, the default skill repository, and the
	// default agent catalog if there are no catalogs.
	// There being no catalogs is a proxy for "has this server been started previously."
	// We don't want to recreate these if an admin deleted them.
	if err := c.List(ctx, &catalogs); err != nil {
		return err
	} else if len(catalogs.Items) == 0 {
		if err := kclient.IgnoreAlreadyExists(c.Create(ctx, &everythingAccessControlRule)); err != nil {
			return err
		}

		var everythingSkillAccessRule v1.SkillAccessRule
		if err := yaml.Unmarshal(everythingSkillAccessRuleData, &everythingSkillAccessRule); err != nil {
			return fmt.Errorf("failed to unmarshal everything skill access rule: %w", err)
		}

		if err := kclient.IgnoreAlreadyExists(c.Create(ctx, &everythingSkillAccessRule)); err != nil {
			return err
		}

		var everythingHostedAgentAccessRule v1.HostedAgentAccessRule
		if err := yaml.Unmarshal(everythingHostedAgentAccessRuleData, &everythingHostedAgentAccessRule); err != nil {
			return fmt.Errorf("failed to unmarshal everything hosted agent access rule: %w", err)
		}

		if err := kclient.IgnoreAlreadyExists(c.Create(ctx, &everythingHostedAgentAccessRule)); err != nil {
			return err
		}

		if err := createDefaultSkillRepository(ctx, c, defaults.SkillRepoURL, defaults.SkillRepoRef); err != nil {
			return err
		}

		if err := createDefaultAgentCatalog(ctx, c, defaults.HostedAgentsCatalogURL, defaults.HostedAgentsCatalogRef, defaults.AllowLocalRepos); err != nil {
			return err
		}
	}

	return nil
}

func createDefaultSkillRepository(ctx context.Context, c kclient.Client, repoURL, ref string) error {
	repoURL = strings.TrimSpace(repoURL)
	ref = strings.TrimSpace(ref)

	if repoURL == "" {
		return nil
	}

	var err error
	repoURL, err = skillrepository.NormalizeRepositoryURL(repoURL)
	if err != nil {
		return fmt.Errorf("invalid default skill repository URL: %w", err)
	}

	return kclient.IgnoreAlreadyExists(c.Create(ctx, &v1.SkillRepository{
		Name:      system.DefaultSkillRepository,
		Namespace: system.DefaultNamespace,
		Spec: v1.SkillRepositorySpec{
			DisplayName: "Default",
			RepoURL:     repoURL,
			Ref:         ref,
		},
	}))
}

// createDefaultAgentCatalog seeds the AgentCatalog that hosted agents and
// harnesses are discovered from, mirroring createDefaultSkillRepository. The
// repository is cloned by the sync handler at runtime, so only the pointer is
// stored here.
func createDefaultAgentCatalog(ctx context.Context, c kclient.Client, repoURL, ref string, allowLocalRepos bool) error {
	repoURL = strings.TrimSpace(repoURL)
	ref = strings.TrimSpace(ref)

	if repoURL == "" {
		return nil
	}

	manifest := types.AgentCatalogManifest{
		DisplayName: "Default",
		RepoURL:     repoURL,
		Ref:         ref,
	}

	// A developer may point this at a local checkout, which is only accepted
	// when Obot itself is in development mode.
	if err := agentcatalog.Validate(manifest, allowLocalRepos); err != nil {
		return fmt.Errorf("invalid default agent catalog: %w", err)
	}

	return kclient.IgnoreAlreadyExists(c.Create(ctx, &v1.AgentCatalog{
		Name:      system.DefaultAgentCatalog,
		Namespace: system.DefaultNamespace,
		Spec: v1.AgentCatalogSpec{
			AgentCatalogManifest: manifest,
		},
	}))
}
