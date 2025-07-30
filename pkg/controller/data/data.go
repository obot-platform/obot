package data

import (
	"context"
	_ "embed"
	"fmt"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

//go:embed default-models.yaml
var defaultModelsData []byte

//go:embed default-model-aliases.yaml
var defaultModelAliasesData []byte

//go:embed everything-access-control-rule.yaml
var everythingAccessControlRuleData []byte

func Data(ctx context.Context, c kclient.Client, agentDir string) error {
	var defaultModels v1.ModelList
	if err := yaml.Unmarshal(defaultModelsData, &defaultModels); err != nil {
		return fmt.Errorf("failed to unmarshal default models: %w", err)
	}

	for _, model := range defaultModels.Items {
		// Delete these old default models
		if err := kclient.IgnoreNotFound(c.Delete(ctx, &model)); err != nil {
			return err
		}
	}

	var defaultModelAliases v1.DefaultModelAliasList
	if err := yaml.Unmarshal(defaultModelAliasesData, &defaultModelAliases); err != nil {
		return fmt.Errorf("failed to unmarshal default model aliases: %w", err)
	}

	for _, alias := range defaultModelAliases.Items {
		var existing v1.DefaultModelAlias
		if err := c.Get(ctx, kclient.ObjectKey{Namespace: alias.Namespace, Name: alias.Name}, &existing); apierrors.IsNotFound(err) {
			if err := c.Create(ctx, &alias); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}

	var everythingAccessControlRule v1.AccessControlRule
	if err := yaml.Unmarshal(everythingAccessControlRuleData, &everythingAccessControlRule); err != nil {
		return fmt.Errorf("failed to unmarshal everything access control rule: %w", err)
	}

	var catalogs v1.MCPCatalogList
	// Only create the "everything" access control rule if there are no catalogs.
	// There being no catalogs is a proxy for "has this server been started previously"
	// We don't want to recreate this access control rule if an admin deleted it.
	if err := c.List(ctx, &catalogs); err == nil && len(catalogs.Items) == 0 {
		if err = kclient.IgnoreAlreadyExists(c.Create(ctx, &everythingAccessControlRule)); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return addAgents(ctx, c, agentDir)
}
