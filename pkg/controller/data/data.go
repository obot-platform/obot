package data

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/handlers/skillrepository"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

//go:embed default-model-aliases.yaml
var defaultModelAliasesData []byte

//go:embed everything-access-control-rule.yaml
var everythingAccessControlRuleData []byte

//go:embed everything-skill-access-rule.yaml
var everythingSkillAccessRuleData []byte

func Data(ctx context.Context, c kclient.Client, agentDir, defaultSkillRepoURL, defaultSkillRepoRef string) error {
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
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.ModelAccessPolicyPrefix + "-default",
				Namespace: system.DefaultNamespace,
			},
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
	// Only seed default access/skill rules and the default skill repository if there are no catalogs.
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

		if err := createDefaultSkillRepository(ctx, c, defaultSkillRepoURL, defaultSkillRepoRef); err != nil {
			return err
		}
	}

	return addAgents(ctx, c, agentDir)
}

func createDefaultSkillRepository(ctx context.Context, c kclient.Client, repoURL, ref string) error {
	repoURL = strings.TrimSpace(repoURL)
	ref = strings.TrimSpace(ref)

	if repoURL == "" {
		return nil
	}

	if err := skillrepository.ValidateRepositoryURL(repoURL); err != nil {
		return fmt.Errorf("invalid default skill repository URL: %w", err)
	}

	return kclient.IgnoreAlreadyExists(c.Create(ctx, &v1.SkillRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.DefaultSkillRepository,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.SkillRepositorySpec{
			SkillRepositoryManifest: types.SkillRepositoryManifest{
				DisplayName: "Default",
				RepoURL:     repoURL,
				Ref:         ref,
			},
		},
	}))
}

