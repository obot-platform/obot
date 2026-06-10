package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const CookieSecretEnvVar = "OBOT_AUTH_PROVIDER_COOKIE_SECRET"

type AuthProviderHandler struct {
	dispatcher  *dispatcher.Dispatcher
	postgresDSN string
	license     *license.KeygenProvider
}

func NewAuthProviderHandler(dispatcher *dispatcher.Dispatcher, postgresDSN string, licenseProvider *license.KeygenProvider) *AuthProviderHandler {
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

	resp, err := ap.convertAuthProvider(authProvider)
	if err != nil {
		return err
	}

	return req.Write(resp)
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
		authProvider, err := ap.convertAuthProvider(a)
		if err != nil {
			log.Warnf("failed to convert auth provider %q: %v", a.Name, err)
			continue
		}
		resp = append(resp, authProvider)
	}

	return req.Write(types.AuthProviderList{Items: resp})
}

func (ap *AuthProviderHandler) Configure(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
	}

	if err := ap.license.RequireEntitlements(authProvider.Spec.RequiredEntitlements); err != nil {
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

	existingIssuer := ""
	if authProvider.Name == GenericOAuthAuthProviderName {
		existing, err := req.GatewayClient.RevealCredential(req.Context(), []string{authProvider.Name, system.GenericAuthProviderCredentialContext}, authProvider.Name)
		if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to reveal existing generic OAuth credential: %w", err)
		}
		if err == nil && existing.Secrets != nil {
			existingIssuer = existing.Secrets[GenericOAuthIssuerEnvVar]
		}
	}

	if err := validateGenericOAuthConfig(req.Context(), authProvider.Name, envVars); err != nil {
		return types.NewErrBadRequest("invalid generic OAuth provider configuration: %v", err)
	}
	if err := requireGenericOAuthTrustReconfirmation(authProvider.Name, existingIssuer, envVars); err != nil {
		return types.NewErrBadRequest("invalid generic OAuth provider configuration: %v", err)
	}

	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: authProvider.Name,
		Name:    authProvider.Name,
		Secrets: envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential for auth provider %q: %w", authProvider.Name, err)
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

	if authProvider.Annotations[v1.AuthProviderSyncAnnotation] == "" {
		if authProvider.Annotations == nil {
			authProvider.Annotations = make(map[string]string, 1)
		}
		authProvider.Annotations[v1.AuthProviderSyncAnnotation] = "true"
	} else {
		delete(authProvider.Annotations, v1.AuthProviderSyncAnnotation)
	}

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

	return nil
}

func (ap *AuthProviderHandler) Deconfigure(req api.Context) error {
	var authProvider v1.AuthProvider
	if err := req.Get(&authProvider, req.PathValue("id")); err != nil {
		return err
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

	if authProvider.Annotations[v1.AuthProviderSyncAnnotation] == "" {
		if authProvider.Annotations == nil {
			authProvider.Annotations = make(map[string]string, 1)
		}
		authProvider.Annotations[v1.AuthProviderSyncAnnotation] = "true"
	} else {
		delete(authProvider.Annotations, v1.AuthProviderSyncAnnotation)
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

	return nil
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

func (ap *AuthProviderHandler) convertAuthProvider(authProvider v1.AuthProvider) (types.AuthProvider, error) {
	authProviderStatus, err := providers.AuthProviderStatus(authProvider, nil, ap.license)
	if err != nil {
		return types.AuthProvider{}, fmt.Errorf("failed to get auth provider status: %w", err)
	}

	return types.AuthProvider{
		Metadata:             MetadataFrom(&authProvider),
		AuthProviderManifest: authProvider.Spec.AuthProviderManifest,
		AuthProviderStatus:   *authProviderStatus,
	}, nil
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
