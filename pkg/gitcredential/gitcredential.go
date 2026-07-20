package gitcredential

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	obotgit "github.com/obot-platform/obot/pkg/git"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	credentialContext = "git-credentials"
	tokenKey          = "token"
)

// NormalizeHost validates and canonicalizes a Git credential host.
func NormalizeHost(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("host is required")
	}
	if strings.Contains(value, "://") || strings.ContainsAny(value, "/?#@") {
		return "", fmt.Errorf("host must not include a scheme, path, query, or user information")
	}

	u, err := url.Parse("https://" + value)
	if err != nil || u.Hostname() == "" {
		return "", fmt.Errorf("invalid host %q", value)
	}
	if port := u.Port(); port != "" {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return "", fmt.Errorf("invalid host port %q", port)
		}
		return strings.ToLower(u.Hostname()) + ":" + port, nil
	}
	return strings.ToLower(u.Hostname()), nil
}

func validateSourceURL(sourceURL, host string) error {
	if !obotgit.IsGitRepoURL(sourceURL) {
		return fmt.Errorf("source URL %q is not a Git repository URL", sourceURL)
	}
	u, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("invalid source URL %q: %w", sourceURL, err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("source URL %q must use HTTPS", sourceURL)
	}
	normalizedSourceHost, err := NormalizeHost(u.Host)
	if err != nil {
		return fmt.Errorf("invalid source URL host: %w", err)
	}
	if !strings.EqualFold(normalizedSourceHost, host) {
		return fmt.Errorf("git credential for host %q cannot be used with source host %q", host, normalizedSourceHost)
	}
	return nil
}

// Store saves the token for the identified Git credential.
func Store(ctx context.Context, gatewayClient *gclient.Client, credentialID, token string) error {
	if token == "" {
		return fmt.Errorf("token is required")
	}
	return gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: credentialContext,
		Name:    credentialID,
		Secrets: map[string]string{tokenKey: token},
	})
}

// Configured reports whether the identified Git credential has a stored token.
func Configured(ctx context.Context, gatewayClient *gclient.Client, credentialID string) (bool, error) {
	credential, err := gatewayClient.RevealCredential(ctx, []string{credentialContext}, credentialID)
	if errors.As(err, &gclient.CredentialNotFoundError{}) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return credential.Secrets[tokenKey] != "", nil
}

// Delete removes the stored token for the identified Git credential.
func Delete(ctx context.Context, gatewayClient *gclient.Client, credentialID string) error {
	_, err := gatewayClient.DeleteCredential(ctx, credentialContext, credentialID)
	return err
}

// Resolve validates a Git credential for a source URL and returns its stored token.
func Resolve(ctx context.Context, storageClient client.Client, gatewayClient *gclient.Client, namespace, credentialID, sourceURL string) (string, error) {
	if credentialID == "" {
		return "", nil
	}

	var credential v1.GitCredential
	if err := storageClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: credentialID}, &credential); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("git credential %q does not exist", credentialID)
		}
		return "", fmt.Errorf("failed to get Git credential %q: %w", credentialID, err)
	}
	if !credential.DeletionTimestamp.IsZero() {
		return "", fmt.Errorf("git credential %q is being deleted", credentialID)
	}
	if err := validateSourceURL(sourceURL, credential.Spec.Host); err != nil {
		return "", err
	}
	storedCredential, err := gatewayClient.RevealCredential(ctx, []string{credentialContext}, credentialID)
	if err != nil {
		return "", fmt.Errorf("failed to reveal Git credential %q: %w", credentialID, err)
	}
	token := storedCredential.Secrets[tokenKey]
	if token == "" {
		return "", fmt.Errorf("git credential %q has no token configured", credentialID)
	}
	return token, nil
}

// ReferencesByCredential lists resources in the namespace that use Git credentials, keyed by credential ID.
func ReferencesByCredential(ctx context.Context, storageClient client.Client, namespace string) (map[string][]string, error) {
	references := map[string][]string{}
	var repositories v1.SkillRepositoryList
	if err := storageClient.List(ctx, &repositories, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list skill repositories: %w", err)
	}
	for _, repository := range repositories.Items {
		if credentialID := repository.Spec.GitCredentialID; credentialID != "" {
			references[credentialID] = append(references[credentialID], "skill repository "+repository.Name)
		}
	}

	var catalogs v1.MCPCatalogList
	if err := storageClient.List(ctx, &catalogs, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list MCP catalogs: %w", err)
	}
	for _, catalog := range catalogs.Items {
		credentialIDs := map[string]struct{}{}
		for _, id := range catalog.Spec.SourceURLGitCredentialIDs {
			if id != "" {
				credentialIDs[id] = struct{}{}
			}
		}
		for credentialID := range credentialIDs {
			references[credentialID] = append(references[credentialID], "MCP catalog "+catalog.Name)
		}
	}

	var systemCatalogs v1.SystemMCPCatalogList
	if err := storageClient.List(ctx, &systemCatalogs, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list system MCP catalogs: %w", err)
	}
	for _, catalog := range systemCatalogs.Items {
		credentialIDs := map[string]struct{}{}
		for _, id := range catalog.Spec.SourceURLGitCredentialIDs {
			if id != "" {
				credentialIDs[id] = struct{}{}
			}
		}
		for credentialID := range credentialIDs {
			references[credentialID] = append(references[credentialID], "system MCP catalog "+catalog.Name)
		}
	}
	return references, nil
}

// References lists resources in the namespace that use the identified Git credential.
func References(ctx context.Context, storageClient client.Client, namespace, credentialID string) ([]string, error) {
	references, err := ReferencesByCredential(ctx, storageClient, namespace)
	if err != nil {
		return nil, err
	}
	return references[credentialID], nil
}
