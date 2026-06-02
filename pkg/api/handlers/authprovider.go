package handlers

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers/providers"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/tidwall/gjson"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

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
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != v1.ToolReferenceTypeAuthProvider {
		return types.NewErrNotFound(
			"auth provider %q not found",
			ref.Name,
		)
	}

	authProvider, err := ap.convertToolReferenceToAuthProvider(ref, nil)
	if err != nil {
		return err
	}

	return req.Write(authProvider)
}

func (ap *AuthProviderHandler) List(req api.Context) error {
	resp, err := ap.listAuthProviders(req)
	if err != nil {
		return err
	}

	return req.Write(types.AuthProviderList{Items: resp})
}

func (ap *AuthProviderHandler) listAuthProviders(req api.Context) ([]types.AuthProvider, error) {
	var refList v1.ToolReferenceList
	if err := req.List(&refList, &kclient.ListOptions{
		Namespace: req.Namespace(),
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(v1.ToolReferenceTypeAuthProvider),
		}),
	}); err != nil {
		return nil, err
	}

	resp := make([]types.AuthProvider, 0, len(refList.Items))
	for _, ref := range refList.Items {
		authProvider, err := ap.convertToolReferenceToAuthProvider(ref, nil)
		if err != nil {
			log.Warnf("failed to convert auth provider %q: %v", ref.Name, err)
			continue
		}
		resp = append(resp, authProvider)
	}
	return resp, nil
}

func (ap *AuthProviderHandler) Configure(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != v1.ToolReferenceTypeAuthProvider {
		return types.NewErrBadRequest("%q is not an auth provider", ref.Name)
	}

	if err := ap.license.RequireForProvider(ref); err != nil {
		return err
	}

	configuredProvider, err := ap.dispatcher.GetConfiguredAuthProvider(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	if configuredProvider != "" && configuredProvider != ref.Name {
		return types.NewErrBadRequest(
			"only one authentication provider can be configured at a time. Please deconfigure %q first",
			configuredProvider,
		)
	}
	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}

	cookieSecret, err := generateCookieSecret()
	if err != nil {
		return err
	}
	envVars[providers.CookieSecretEnvVar] = cookieSecret

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericAuthProviderCredentialContext}, ref.Name)
	if err != nil {
		if !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if _, err = req.GatewayClient.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	for key, val := range envVars {
		if val == "" {
			delete(envVars, key)
		}
	}

	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: string(ref.UID),
		Name:    ref.Name,
		Secrets: envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential for auth provider %q: %w", ref.Name, err)
	}

	ap.dispatcher.StopAuthProvider(ref.Namespace, ref.Name)

	// Check to make sure that only this provider is configured.
	// Deconfigure it if that is not the case, and return a 400.
	configuredProvider, err = ap.dispatcher.GetConfiguredAuthProvider(req.Context())
	if err != nil {
		return fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	if configuredProvider != "" && configuredProvider != ref.Name {
		// Delete the credential we just configured
		_, _ = req.GatewayClient.DeleteCredential(req.Context(), string(ref.UID), ref.Name)
		return types.NewErrBadRequest(
			"only one authentication provider can be configured at a time. Please deconfigure %q first",
			configuredProvider,
		)
	}
	if ref.Annotations[v1.AuthProviderSyncAnnotation] == "" {
		if ref.Annotations == nil {
			ref.Annotations = make(map[string]string, 1)
		}
		ref.Annotations[v1.AuthProviderSyncAnnotation] = "true"
	} else {
		delete(ref.Annotations, v1.AuthProviderSyncAnnotation)
	}

	return req.Update(&ref)
}

func (ap *AuthProviderHandler) Deconfigure(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != v1.ToolReferenceTypeAuthProvider {
		return types.NewErrBadRequest("%q is not an auth provider", ref.Name)
	}

	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericAuthProviderCredentialContext}, ref.Name)
	if err != nil {
		if !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to find credential: %w", err)
		}
	} else if _, err = req.GatewayClient.DeleteCredential(req.Context(), cred.Context, ref.Name); err != nil {
		return fmt.Errorf("failed to remove existing credential: %w", err)
	}

	// Stop the auth provider so that the credential is completely removed from the system.
	ap.dispatcher.StopAuthProvider(ref.Namespace, ref.Name)

	if ref.Annotations[v1.AuthProviderSyncAnnotation] == "" {
		if ref.Annotations == nil {
			ref.Annotations = make(map[string]string, 1)
		}
		ref.Annotations[v1.AuthProviderSyncAnnotation] = "true"
	} else {
		delete(ref.Annotations, v1.AuthProviderSyncAnnotation)
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

		if meta, ok := ref.Status.Tool.Metadata["providerMeta"]; ok {
			tablePrefix := gjson.Get(meta, "postgresTablePrefix").String()
			if tablePrefix != "" {
				if err := db.Exec("DROP TABLE IF EXISTS " + tablePrefix + "sessions;").Error; err != nil {
					return fmt.Errorf("failed to drop sessions table: %w", err)
				}
				if err := db.Exec("DROP TABLE IF EXISTS " + tablePrefix + "session_locks;").Error; err != nil {
					return fmt.Errorf("failed to drop session_locks table: %w", err)
				}
			}
		}
	}

	return req.Update(&ref)
}

func (ap *AuthProviderHandler) Reveal(req api.Context) error {
	var ref v1.ToolReference
	if err := req.Get(&ref, req.PathValue("id")); err != nil {
		return err
	}

	if ref.Spec.Type != v1.ToolReferenceTypeAuthProvider {
		return types.NewErrBadRequest("%q is not an auth provider", ref.Name)
	}

	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{string(ref.UID), system.GenericAuthProviderCredentialContext}, ref.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to reveal credential for auth provider %q: %w", ref.Name, err)
	} else if err == nil {
		return req.Write(cred.Secrets)
	}

	return types.NewErrNotFound("no credential found for %q", ref.Name)
}

func authProviderNameFromToolRef(ref v1.ToolReference) string {
	name := ref.Name
	if ref.Status.Tool != nil {
		name = ref.Status.Tool.Name
	}
	return name
}

func (ap *AuthProviderHandler) convertToolReferenceToAuthProvider(ref v1.ToolReference, credEnvVars map[string]string) (types.AuthProvider, error) {
	aps, err := providers.ConvertAuthProviderToolRef(ref, credEnvVars, ap.license)
	if err != nil {
		return types.AuthProvider{}, err
	}
	authProvider := types.AuthProvider{
		Metadata: MetadataFrom(&ref),
		AuthProviderManifest: types.AuthProviderManifest{
			Name:          authProviderNameFromToolRef(ref),
			Namespace:     ref.Namespace,
			ToolReference: ref.Spec.Reference,
		},
		AuthProviderStatus: *aps,
	}

	authProvider.Type = "authprovider"

	return authProvider, nil
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
