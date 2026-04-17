package controller

import (
	"context"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func addCatalogIDToAccessControlRules(ctx context.Context, client kclient.Client) error {
	var acRules v1.AccessControlRuleList
	if err := client.List(ctx, &acRules); err != nil {
		return err
	}

	// Iterate over each AccessControlRule and add CatalogID
	for _, acRule := range acRules.Items {
		if acRule.Spec.MCPCatalogID == "" && acRule.Spec.PowerUserWorkspaceID == "" {
			acRule.Spec.MCPCatalogID = system.DefaultCatalog
			if err := client.Update(ctx, &acRule); err != nil {
				return err
			}
		}
	}

	return nil
}

func migratePublishedArtifactVisibility(ctx context.Context, client kclient.Client) error {
	var artifacts v1.PublishedArtifactList
	if err := client.List(ctx, &artifacts); err != nil {
		return err
	}

	for i := range artifacts.Items {
		artifact := &artifacts.Items[i]
		if artifact.Spec.LegacyVisibility == "" {
			continue
		}

		var subjects []types.Subject
		switch artifact.Spec.LegacyVisibility {
		case "public":
			subjects = []types.Subject{{
				Type: types.SubjectTypeSelector,
				ID:   "*",
			}}
		case "private":
			subjects = nil
		default:
			log.Errorf("invalid legacy visibility %q for published artifact %s", artifact.Spec.LegacyVisibility, artifact.Name)
			// Make it private to be safe
			subjects = nil
		}

		for j := range artifact.Status.Versions {
			artifact.Status.Versions[j].Subjects = subjects
		}

		artifact.Spec.LegacyVisibility = ""
		if err := client.Update(ctx, artifact); err != nil {
			return err
		}
	}

	return nil
}
