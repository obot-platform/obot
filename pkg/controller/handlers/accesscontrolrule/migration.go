package accesscontrolrule

import (
	"context"
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Migration annotation to track completion
	migrationAnnotation = "obot.ai/acr-catalog-migration"
	migrationVersion    = "v1"
)

// MigrateToDefaultCatalog migrates existing AccessControlRules to the default catalog
func (h *Handler) MigrateToDefaultCatalog(req router.Request, _ router.Response) error {
	acr := req.Object.(*v1.AccessControlRule)

	// Skip if already migrated
	if acr.Annotations != nil && acr.Annotations[migrationAnnotation] == migrationVersion {
		return nil
	}

	// Skip if already has a catalog ID set
	if acr.Spec.Manifest.MCPCatalogID != "" {
		// Mark as migrated since it already has catalog ID
		if acr.Annotations == nil {
			acr.Annotations = make(map[string]string)
		}
		acr.Annotations[migrationAnnotation] = migrationVersion
		return req.Client.Update(req.Ctx, acr)
	}

	// Verify default catalog exists
	var defaultCatalog v1.MCPCatalog
	if err := req.Client.Get(req.Ctx, client.ObjectKey{
		Name:      system.DefaultCatalog,
		Namespace: system.DefaultNamespace,
	}, &defaultCatalog); err != nil {
		return fmt.Errorf("default catalog not found, cannot migrate access control rule: %w", err)
	}

	// Set the catalog ID to default
	acr.Spec.Manifest.MCPCatalogID = system.DefaultCatalog

	// Add migration annotation
	if acr.Annotations == nil {
		acr.Annotations = make(map[string]string)
	}
	acr.Annotations[migrationAnnotation] = migrationVersion

	// Update the access control rule
	if err := req.Client.Update(req.Ctx, acr); err != nil {
		return fmt.Errorf("failed to migrate access control rule to default catalog: %w", err)
	}

	return nil
}

// CheckMigrationStatus checks if all AccessControlRules have been migrated
func CheckMigrationStatus(ctx context.Context, c client.Client) (bool, error) {
	var acrList v1.AccessControlRuleList
	if err := c.List(ctx, &acrList, &client.ListOptions{
		Namespace: system.DefaultNamespace,
	}); err != nil {
		return false, fmt.Errorf("failed to list access control rules: %w", err)
	}

	for _, acr := range acrList.Items {
		// Check if rule needs migration (no catalog ID and no migration annotation)
		if acr.Spec.Manifest.MCPCatalogID == "" {
			if acr.Annotations == nil || acr.Annotations[migrationAnnotation] != migrationVersion {
				return false, nil
			}
		}
	}

	return true, nil
}