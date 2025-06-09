package usercatalogauthorization

import (
	"context"

	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func GetAuthorizationsForUser(ctx context.Context, c storage.Client, namespace, userID string) ([]v1.UserCatalogAuthorization, error) {
	var authorizations v1.UserCatalogAuthorizationList
	if err := c.List(ctx, &authorizations, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
		"spec.userID": userID,
	}); err != nil {
		return nil, err
	}

	// Also list authorizations for all users.
	var allUsersAuthorizations v1.UserCatalogAuthorizationList
	if err := c.List(ctx, &allUsersAuthorizations, kclient.MatchingFields{
		"spec.userID": "*",
	}); err != nil {
		return nil, err
	}

	return append(authorizations.Items, allUsersAuthorizations.Items...), nil
}

func GetAuthorizationsForCatalog(ctx context.Context, c storage.Client, namespace, catalogName string) ([]v1.UserCatalogAuthorization, error) {
	var authorizations v1.UserCatalogAuthorizationList
	if err := c.List(ctx, &authorizations, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
		"spec.mcpCatalogName": catalogName,
	}); err != nil {
		return nil, err
	}
	return authorizations.Items, nil
}

// GetUserAuthorizationsForCatalog cannot be called with a cached client, as cached clients do not support more than one field selector.
func GetUserAuthorizationsForCatalog(ctx context.Context, c storage.Client, namespace, userID, catalogName string) ([]v1.UserCatalogAuthorization, error) {
	var authorizations v1.UserCatalogAuthorizationList
	if err := c.List(ctx, &authorizations, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
		"spec.userID":         userID,
		"spec.mcpCatalogName": catalogName,
	}); err != nil {
		return nil, err
	}

	// Also list authorizations for all users.
	var allUsersAuthorizations v1.UserCatalogAuthorizationList
	if err := c.List(ctx, &allUsersAuthorizations, kclient.MatchingFields{
		"spec.userID":         "*",
		"spec.mcpCatalogName": catalogName,
	}); err != nil {
		return nil, err
	}

	return append(authorizations.Items, allUsersAuthorizations.Items...), nil
}
