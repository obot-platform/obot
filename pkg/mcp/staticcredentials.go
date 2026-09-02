package mcp

import (
	"context"
	"errors"

	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	catalogEntryStaticCredentialContextPrefix       = "mcp-server-catalog-entry/"
	systemCatalogEntryStaticCredentialContextPrefix = "system-mcp-server-catalog-entry/"
)

// CatalogEntryStaticCredentialContext isolates regular catalog-entry credentials
// from other resource types that may legally have the same name.
func CatalogEntryStaticCredentialContext(resourceName string) string {
	if resourceName == "" {
		return ""
	}
	return catalogEntryStaticCredentialContextPrefix + resourceName
}

// SystemCatalogEntryStaticCredentialContext isolates system catalog-entry
// credentials from other resource types that may legally have the same name.
func SystemCatalogEntryStaticCredentialContext(resourceName string) string {
	if resourceName == "" {
		return ""
	}
	return systemCatalogEntryStaticCredentialContextPrefix + resourceName
}

// StaticCredentialSecrets returns encrypted static configuration for a resource.
// A missing credential is represented by an empty map.
func StaticCredentialSecrets(ctx context.Context, client *gateway.Client, credentialContext, resourceName string) (map[string]string, error) {
	credential, err := client.RevealCredential(ctx, []string{credentialContext}, StaticConfigurationCredentialName(resourceName))
	if err != nil {
		if errors.As(err, &gateway.CredentialNotFoundError{}) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	return credential.Secrets, nil
}

// StoreStaticCredentialSecrets replaces encrypted static configuration for a resource.
// Empty configuration removes the credential.
func StoreStaticCredentialSecrets(ctx context.Context, client *gateway.Client, credentialContext, resourceName string, secrets map[string]string) error {
	credentialName := StaticConfigurationCredentialName(resourceName)
	if len(secrets) == 0 {
		_, err := client.DeleteCredential(ctx, credentialContext, credentialName)
		return err
	}
	return client.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: credentialContext,
		Name:    credentialName,
		Secrets: secrets,
	})
}

// RuntimeCredentialSecrets combines caller-owned and static configuration while
// keeping the two encrypted credentials independent at rest.
func RuntimeCredentialSecrets(ctx context.Context, client *gateway.Client, credentialContext, resourceName string) (map[string]string, error) {
	var credCtx []string
	if credentialContext != "" {
		credCtx = []string{credentialContext}
	}
	user, err := client.RevealCredential(ctx, credCtx, resourceName)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return nil, err
	}
	static, err := StaticCredentialSecrets(ctx, client, credentialContext, resourceName)
	if err != nil {
		return nil, err
	}
	return MergeRuntimeConfiguration(user.Secrets, static), nil
}
