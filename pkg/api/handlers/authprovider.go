package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/localauth"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	CookieSecretEnvVar = "OBOT_AUTH_PROVIDER_COOKIE_SECRET"
)

type AuthProviderHandler struct {
	dispatcher  *dispatcher.Dispatcher
	postgresDSN string
	license     *license.Provider
}

func NewAuthProviderHandler(dispatcher *dispatcher.Dispatcher, postgresDSN string, licenseProvider *license.Provider) *AuthProviderHandler {
	return &AuthProviderHandler{
		dispatcher:  dispatcher,
		postgresDSN: postgresDSN,
		license:     licenseProvider,
	}
}

func (ap *AuthProviderHandler) ByID(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	authProviderStatus, err := providers.AuthProviderStatus(req.Context(), authProvider, nil, ap.license)
	if err != nil {
		return err
	}

	return req.Write(ap.convertAuthProvider(authProvider, *authProviderStatus))
}

func (ap *AuthProviderHandler) List(req api.Context) error {
	var authProviders v1.AuthProviderList
	if err := req.List(&authProviders, &kclient.ListOptions{
		Namespace: req.Namespace(),
	}); err != nil {
		return err
	}

	resp := make([]types.AuthProvider, 0, len(authProviders.Items))
	for _, a := range authProviders.Items {
		authProviderStatus, err := providers.AuthProviderStatus(req.Context(), a, nil, ap.license)
		if err != nil {
			return err
		}

		resp = append(resp, ap.convertAuthProvider(a, *authProviderStatus))
	}

	return req.Write(types.AuthProviderList{Items: resp})
}

func (ap *AuthProviderHandler) Configure(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	if err := ap.license.RequireEntitlements(req.Context(), authProvider.Spec.RequiredEntitlements); err != nil {
		return err
	}

	if err := ensureNoPendingAuthProviderCleanup(req, authProvider); err != nil {
		return err
	}

	configuredProvider, err := ap.dispatcher.GetConfiguredAuthProvider(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	if configuredProvider != "" && configuredProvider != authProvider.Name {
		return types.NewErrBadRequest(
			"only one authentication provider can be configured at a time. Please deconfigure %q first",
			configuredProvider,
		)
	}
	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	} else if envVars == nil {
		envVars = make(map[string]string, 1)
	}

	envVars[CookieSecretEnvVar], err = generateCookieSecret()
	if err != nil {
		return err
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: authProvider.Name,
		Name:    authProvider.Name,
		Secrets: envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential for auth provider %q: %w", authProvider.Name, err)
	}
	if err := ensureNoPendingAuthProviderCleanup(req, authProvider); err != nil {
		return ap.rollbackAuthProviderConfiguration(req, authProvider, err)
	}

	ap.dispatcher.StopAuthProvider(authProvider.Namespace, authProvider.Name)

	// Check to make sure that only this provider is configured.
	// Deconfigure it if that is not the case, and return a 400.
	configuredProvider, err = ap.dispatcher.GetConfiguredAuthProvider(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get configured auth provider: %w", err)
	}

	if configuredProvider != "" && configuredProvider != authProvider.Name {
		// Delete the credential we just configured
		_, _ = req.GatewayClient.DeleteCredential(req.Context(), authProvider.Name, authProvider.Name)
		return types.NewErrBadRequest(
			"only one authentication provider can be configured at a time. Please deconfigure %q first",
			configuredProvider,
		)
	}

	setAuthProviderSyncRevision(&authProvider)

	if err := req.Update(&authProvider); err != nil {
		return fmt.Errorf("failed to update auth provider: %w", err)
	}

	// Wait for the controllers to process to ensure the API will return correct configuration status.
	if _, err := wait.For(req.Context(), req.Storage, &authProvider, func(a *v1.AuthProvider) (bool, error) {
		return a.Status.ObservedGeneration == a.Generation, nil
	}, wait.Option{
		Timeout: 10 * time.Second,
	}); err != nil {
		return fmt.Errorf("failed to wait for auth provider: %w", err)
	}
	if err := ensureNoPendingAuthProviderCleanup(req, authProvider); err != nil {
		return ap.rollbackAuthProviderConfiguration(req, authProvider, err)
	}

	return nil
}

func (ap *AuthProviderHandler) rollbackAuthProviderConfiguration(req api.Context, authProvider v1.AuthProvider, cause error) error {
	ap.dispatcher.StopAuthProvider(authProvider.Namespace, authProvider.Name)
	if _, err := req.GatewayClient.DeleteCredential(req.Context(), authProvider.Name, authProvider.Name); err != nil {
		return errors.Join(cause, fmt.Errorf("roll back credential for auth provider %q: %w", authProvider.Name, err))
	}
	return cause
}

func ensureNoPendingAuthProviderCleanup(req api.Context, authProvider v1.AuthProvider) error {
	var cleanups v1.AuthProviderCleanupList
	if err := req.List(&cleanups); err != nil {
		return fmt.Errorf("list pending auth provider cleanups: %w", err)
	}
	for _, cleanup := range cleanups.Items {
		sameProvider := cleanup.Spec.AuthProviderName == authProvider.Name
		samePrefix := authProvider.Spec.GroupIDPrefix != "" && cleanup.Spec.GroupIDPrefix == authProvider.Spec.GroupIDPrefix
		if sameProvider || samePrefix {
			return types.NewErrBadRequest("authentication provider %q is still being deconfigured; wait for cleanup to finish before configuring it again", authProvider.Name)
		}
	}
	return nil
}

func (ap *AuthProviderHandler) Deconfigure(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	var cleanup *v1.AuthProviderCleanup
	if authProvider.Spec.GroupIDPrefix != "" {
		var err error
		cleanup, err = ensureAuthProviderCleanup(req, authProvider)
		if err != nil {
			return err
		}
		if cleanup.Spec.Ready {
			return nil
		}
	}

	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{authProvider.Name, system.GenericAuthProviderCredentialContext}, authProvider.Name)
	if err != nil {
		if !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if _, err = req.GatewayClient.DeleteCredential(req.Context(), cred.Context, authProvider.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	// Stop the auth provider so that the credential is completely removed from the system.
	ap.dispatcher.StopAuthProvider(authProvider.Namespace, authProvider.Name)

	// The local auth provider keeps its sessions in Obot's database rather than in the sessions
	// table the block below drops, so clear them out here.
	if authProvider.Name == localauth.ProviderName {
		if err := req.GatewayClient.DeleteAllLocalAuthSessions(req.Context()); err != nil {
			return fmt.Errorf("failed to delete local auth sessions: %w", err)
		}
	}

	// Drop the sessions table and session_locks table from the database, if it exists.
	if ap.postgresDSN != "" {
		db, err := gorm.Open(postgres.Open(ap.postgresDSN), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			return fmt.Errorf("failed to connect to postgres: %w", err)
		}
		sqlDB, err := db.DB()
		if err != nil {
			return fmt.Errorf("failed to get underlying sql.DB: %w", err)
		}
		defer sqlDB.Close()

		if tablePrefix := authProvider.Spec.PostgresTablePrefix; tablePrefix != "" {
			if err := db.Exec("DROP TABLE IF EXISTS " + tablePrefix + "sessions;").Error; err != nil {
				return fmt.Errorf("failed to drop sessions table: %w", err)
			}
			if err := db.Exec("DROP TABLE IF EXISTS " + tablePrefix + "session_locks;").Error; err != nil {
				return fmt.Errorf("failed to drop session_locks table: %w", err)
			}
		}
	}

	updatedAuthProvider, err := updateAuthProviderForDeconfiguration(req, authProvider.Name)
	if err != nil {
		return err
	}

	// Wait for the controllers to process to ensure the API will return correct configuration status.
	if updatedAuthProvider != nil {
		if _, err := wait.For(req.Context(), req.Storage, updatedAuthProvider, func(a *v1.AuthProvider) (bool, error) {
			return a.Status.ObservedGeneration == a.Generation, nil
		}, wait.Option{
			Timeout: 10 * time.Second,
		}); err != nil {
			return fmt.Errorf("failed to wait for auth provider: %w", err)
		}
	}

	if cleanup != nil {
		if err := markAuthProviderCleanupReady(req, cleanup); err != nil {
			return err
		}
	}

	return nil
}

func authProviderCleanupName(authProviderName string) string {
	return name.SafeConcatName(system.AuthProviderCleanupPrefix, authProviderName)
}

func ensureAuthProviderCleanup(req api.Context, authProvider v1.AuthProvider) (*v1.AuthProviderCleanup, error) {
	cleanup := &v1.AuthProviderCleanup{}
	cleanupName := authProviderCleanupName(authProvider.Name)
	if err := req.Get(cleanup, cleanupName); err == nil {
		if cleanup.Spec.AuthProviderName != authProvider.Name || cleanup.Spec.GroupIDPrefix != authProvider.Spec.GroupIDPrefix {
			return nil, fmt.Errorf("auth provider cleanup %q does not match auth provider %q", cleanupName, authProvider.Name)
		}
		return cleanup, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("get auth provider cleanup %q: %w", cleanupName, err)
	}

	cleanup = &v1.AuthProviderCleanup{
		Name:      cleanupName,
		Namespace: authProvider.Namespace,
		Spec: v1.AuthProviderCleanupSpec{
			AuthProviderName: authProvider.Name,
			GroupIDPrefix:    authProvider.Spec.GroupIDPrefix,
		},
	}
	if err := req.Create(cleanup); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return ensureAuthProviderCleanup(req, authProvider)
		}
		return nil, fmt.Errorf("persist cleanup intent for auth provider %q: %w", authProvider.Name, err)
	}
	return cleanup, nil
}

func updateAuthProviderForDeconfiguration(req api.Context, authProviderName string) (*v1.AuthProvider, error) {
	var authProvider v1.AuthProvider
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := req.Get(&authProvider, authProviderName); err != nil {
			return err
		}
		setAuthProviderSyncRevision(&authProvider)
		return req.Update(&authProvider)
	})
	if apierrors.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update auth provider for deconfiguration: %w", err)
	}
	return &authProvider, nil
}

func markAuthProviderCleanupReady(req api.Context, cleanup *v1.AuthProviderCleanup) error {
	var current v1.AuthProviderCleanup
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := req.Get(&current, cleanup.Name); err != nil {
			return err
		}
		if current.Spec.Ready {
			return nil
		}
		current.Spec.Ready = true
		return req.Update(&current)
	})
	if apierrors.IsNotFound(err) {
		// Another deconfigure request may have marked the task ready and the controller may
		// have completed it before this request observed the update.
		return nil
	}
	if err != nil {
		return fmt.Errorf("mark auth provider cleanup %q ready: %w", cleanup.Name, err)
	}
	return nil
}

func setAuthProviderSyncRevision(authProvider *v1.AuthProvider) {
	if authProvider.Annotations == nil {
		authProvider.Annotations = make(map[string]string, 1)
	}
	authProvider.Annotations[v1.AuthProviderSyncAnnotation] = uuid.New().String()
}

func (ap *AuthProviderHandler) Reveal(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{authProvider.Name, system.GenericAuthProviderCredentialContext}, authProvider.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to reveal credential for auth provider %q: %w", authProvider.Name, err)
	} else if err == nil {
		return req.Write(cred.Secrets)
	}

	return types.NewErrNotFound("no credential found for %q", authProvider.Name)
}

func (ap *AuthProviderHandler) convertAuthProvider(authProvider v1.AuthProvider, authProviderStatus types.AuthProviderStatus) types.AuthProvider {
	return types.AuthProvider{
		Metadata:             MetadataFrom(&authProvider),
		AuthProviderManifest: authProvider.Spec.AuthProviderManifest,
		AuthProviderStatus:   authProviderStatus,
	}
}

func generateCookieSecret() (string, error) {
	const length = 32

	// Generate a random token. Repeat until we have one that is 32 bytes long after trimming.
	// This only takes one try in the vast majority of circumstances, but could occasionally take a second.
	var b = make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	for len(bytes.TrimSpace(b)) != length {
		_, err := rand.Read(b)
		if err != nil {
			return "", fmt.Errorf("failed to generate random token: %w", err)
		}
	}

	return base64.StdEncoding.EncodeToString(b), nil
}
