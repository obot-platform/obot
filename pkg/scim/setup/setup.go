// Package setup creates the SCIM connection of an auth provider and keeps the provider's status consistent with it.
package setup

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/pkg/controller/handlers/provider"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Options describe the SCIM connection to create.
type Options struct {
	// AuthProviderName names the auth provider, in the default namespace, whose users and groups the connection
	// manages.
	AuthProviderName string
	Origin           types.SCIMConnectionOrigin
	// Issuer is the identity provider's issuer URL. When it is empty, it is read from the auth provider's stored
	// credential.
	Issuer string
	// IssueToken issues the connection's first bearer token.
	IssueToken bool
}

// StatusError reports that a SCIM connection was created, but its auth provider's status could not be recomputed.
type StatusError struct {
	AuthProviderName string
	Err              error
}

// CleanupPendingError reports that a SCIM connection was not created, because an auth-provider cleanup is pending for
// the auth provider or its group ID prefix.
type CleanupPendingError struct {
	AuthProviderName string
	CleanupName      string
}

// CreateConnection creates the SCIM connection of an auth provider, and returns it with its bearer token when
// opts.IssueToken is set. It then recomputes the provider's status, because the connection changes which parameters
// the provider needs and the controller does not watch the gateway database.
//
// It refuses to create the connection while an auth-provider cleanup is pending for the provider's name or group ID
// prefix. A cleanup checks for a connection only while it deletes the gateway data, and strips the policy subjects
// afterward, so it would strip the subjects of a connection created in between. Only provider configuration changes
// create cleanups, so the refusal is complete when the caller is serialized with them.
func CreateConnection(ctx context.Context, storage kclient.Client, gateway *gclient.Client, licenseProvider *license.Provider, opts Options) (*types.SCIMConnection, string, error) {
	var authProvider v1.AuthProvider
	if err := storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: opts.AuthProviderName}, &authProvider); err != nil {
		return nil, "", fmt.Errorf("failed to get auth provider %q: %w", opts.AuthProviderName, err)
	}
	if !adapter.SupportsSCIM(authProvider.Name, authProvider.Spec.AuthProviderManifest) {
		return nil, "", fmt.Errorf("auth provider %q does not support SCIM", authProvider.Name)
	}
	if err := refusePendingCleanup(ctx, storage, authProvider); err != nil {
		return nil, "", err
	}

	issuer := opts.Issuer
	if issuer == "" {
		var err error
		if issuer, err = storedIssuer(ctx, gateway, authProvider); err != nil {
			return nil, "", err
		}
	}

	conn, token, err := gateway.CreateSCIMConnection(ctx, gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: authProvider.Namespace,
		AuthProviderName:      authProvider.Name,
		GroupIDPrefix:         authProvider.Spec.GroupIDPrefix,
		Issuer:                issuer,
		Origin:                opts.Origin,
		IssueToken:            opts.IssueToken,
	})
	if err != nil {
		return nil, "", err
	}

	// The connection exists now, and its token cannot be shown again, so failing to record the provider's new status
	// does not fail the creation. The next reconcile of the auth provider records it.
	if err := RecomputeAuthProviderStatus(ctx, storage, gateway, licenseProvider, authProvider.Namespace, authProvider.Name); err != nil {
		return conn, token, &StatusError{
			AuthProviderName: authProvider.Name,
			Err:              err,
		}
	}

	return conn, token, nil
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("failed to recompute the status of auth provider %q: %v", e.AuthProviderName, e.Err)
}

func (e *StatusError) Unwrap() error {
	return e.Err
}

func (e *CleanupPendingError) Error() string {
	return fmt.Sprintf("auth provider cleanup %q is pending, so auth provider %q cannot have a SCIM connection until it finishes", e.CleanupName, e.AuthProviderName)
}

// RecomputeAuthProviderStatus recomputes and stores the status of an auth provider, which depends on its SCIM
// connection.
func RecomputeAuthProviderStatus(ctx context.Context, storage kclient.Client, gateway *gclient.Client, licenseProvider *license.Provider, namespace, name string) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var authProvider v1.AuthProvider
		if err := storage.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: name}, &authProvider); err != nil {
			return err
		}
		if err := provider.SetAuthProviderConfiguredStatus(ctx, gateway, licenseProvider, &authProvider); err != nil {
			return err
		}
		return storage.Status().Update(ctx, &authProvider)
	})
}

// refusePendingCleanup returns a CleanupPendingError when an auth-provider cleanup is pending for the auth provider's
// name or group ID prefix, the same data that a cleanup refuses to delete while a connection owns it.
func refusePendingCleanup(ctx context.Context, storage kclient.Client, authProvider v1.AuthProvider) error {
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
