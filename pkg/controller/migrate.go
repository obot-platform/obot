package controller

import (
	"context"

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
		if acRule.Spec.MCPCatalogID == "" {
			acRule.Spec.MCPCatalogID = system.DefaultCatalog
			if err := client.Update(ctx, &acRule); err != nil {
				return err
			}
		}
	}

	return nil
}
