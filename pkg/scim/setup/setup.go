// Package setup sets up and administers the SCIM connections of auth providers.
package setup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// CleanupPendingError reports that a SCIM connection was not created, because an auth-provider cleanup is pending for
// the auth provider or its group ID prefix.
type CleanupPendingError struct {
	AuthProviderName string
	CleanupName      string
}

func (e *CleanupPendingError) Error() string {
	return fmt.Sprintf("auth provider cleanup %q is pending, so auth provider %q cannot have a SCIM connection until it finishes", e.CleanupName, e.AuthProviderName)
}

// refusePendingCleanup returns a CleanupPendingError when an auth-provider cleanup is pending for the auth provider's
// name or group ID prefix, the same data that a cleanup refuses to delete while a connection owns it.
func refusePendingCleanup(ctx context.Context, storage kclient.Reader, authProvider v1.AuthProvider) error {
	var cleanups v1.AuthProviderCleanupList
	if err := storage.List(ctx, &cleanups, kclient.InNamespace(authProvider.Namespace)); err != nil {
		return fmt.Errorf("failed to list auth provider cleanups: %w", err)
	}
	for _, cleanup := range cleanups.Items {
		if cleanup.Spec.AuthProviderName == authProvider.Name || cleanup.Spec.GroupIDPrefix == authProvider.Spec.GroupIDPrefix {
			return &CleanupPendingError{
				AuthProviderName: authProvider.Name,
				CleanupName:      cleanup.Name,
			}
		}
	}
	return nil
}

// storedIssuer reads the identity provider's issuer URL from the auth provider's stored credential.
func storedIssuer(ctx context.Context, gateway *gclient.Client, authProvider v1.AuthProvider) (string, error) {
	a, _ := adapter.ForAuthProvider(authProvider.Name)

	cred, err := gateway.RevealCredential(ctx, []string{authProvider.Name, system.GenericAuthProviderCredentialContext}, authProvider.Name)
	if err != nil && !errors.As(err, &gclient.CredentialNotFoundError{}) {
		return "", fmt.Errorf("failed to reveal the credential of auth provider %q: %w", authProvider.Name, err)
	}
	return strings.TrimSuffix(strings.TrimSpace(cred.Secrets[a.IssuerConfigurationParameter()]), "/"), nil
}
