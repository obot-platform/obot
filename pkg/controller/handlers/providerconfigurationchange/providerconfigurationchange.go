package providerconfigurationchange

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/controller/handlers/provider"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/localauth"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// OrphanedStagedCredentialGracePeriod protects credentials staged immediately
	// before their ProviderConfigurationChange is persisted.
	OrphanedStagedCredentialGracePeriod = 5 * time.Minute
)

type Handler struct {
	gatewayClient   *gateway.Client
	dispatcher      *dispatcher.Dispatcher
	licenseProvider *license.Provider
	postgresDSN     string
}

type authProviderConflictError struct {
	configuredProvider string
}

func (e *authProviderConflictError) Error() string {
	return fmt.Sprintf("only one authentication provider can be configured at a time. Please deconfigure %q first", e.configuredProvider)
}

func New(gatewayClient *gateway.Client, dispatcher *dispatcher.Dispatcher, licenseProvider *license.Provider, postgresDSN string) *Handler {
	return &Handler{
		gatewayClient:   gatewayClient,
		dispatcher:      dispatcher,
		licenseProvider: licenseProvider,
		postgresDSN:     postgresDSN,
	}
}

func (h *Handler) Reconcile(req router.Request, _ router.Response) error {
	change := req.Object.(*v1.ProviderConfigurationChange)

	if change.Status.Applied || change.Status.Error != "" {
		if change.Spec.StagedCredentialName != "" {
			if _, err := h.gatewayClient.DeleteCredential(req.Ctx, system.StagedProviderCredentialContext, change.Spec.StagedCredentialName); err != nil {
				return fmt.Errorf("delete staged provider credential %q: %w", change.Spec.StagedCredentialName, err)
			}
		}
		return req.Delete(change)
	}

	if err := validateChange(change); err != nil {
		// The next reconciliation will delete the PCC.
		change.Status.Error = err.Error()
		return nil
	}

	var stagedSecrets map[string]string
	if change.Spec.DesiredState == v1.ProviderDesiredStateConfigured {
		credential, err := h.gatewayClient.RevealCredential(req.Ctx, []string{system.StagedProviderCredentialContext}, change.Spec.StagedCredentialName)
		if err != nil {
			return fmt.Errorf("reveal staged provider credential %q: %w", change.Spec.StagedCredentialName, err)
		}
		stagedSecrets = credential.Secrets
	}

	switch change.Spec.ProviderType {
	case v1.ProviderTypeAuth:
		if err := h.reconcileAuthProvider(req.Ctx, req.Client, change, stagedSecrets); err != nil {
			if conflictErr, ok := errors.AsType[*authProviderConflictError](err); ok {
				change.Status.Error = conflictErr.Error()
				return nil
			}
			return err
		}
	case v1.ProviderTypeModel:
		if err := h.reconcileModelProvider(req.Ctx, req.Client, change, stagedSecrets); err != nil {
			return err
		}
	}

	if err := h.advanceDaemonSync(req.Ctx, req.Client, change); err != nil {
		return err
	}

	change.Status.Applied = true
	return nil
}

func validateChange(change *v1.ProviderConfigurationChange) error {
	if change.Spec.ProviderName == "" {
		return fmt.Errorf("provider configuration change %q has no provider name", change.Name)
	}

	var expectedName string
	switch change.Spec.ProviderType {
	case v1.ProviderTypeAuth:
		expectedName = system.ProviderChangeAuthName
	case v1.ProviderTypeModel:
		expectedName = name.SafeConcatName(system.ProviderChangePrefix, change.Spec.ProviderName)
	default:
		return fmt.Errorf("provider configuration change %q has invalid provider type %q", change.Name, change.Spec.ProviderType)
	}
	if change.Name != expectedName {
		return fmt.Errorf("provider configuration change %q must be named %q", change.Name, expectedName)
	}

	switch change.Spec.DesiredState {
	case v1.ProviderDesiredStateConfigured:
		if change.Spec.StagedCredentialName == "" {
			return fmt.Errorf("provider configuration change %q has no staged credential", change.Name)
		}
	case v1.ProviderDesiredStateDeconfigured:
		if change.Spec.StagedCredentialName != "" {
			return fmt.Errorf("deconfiguration change %q must not reference a staged credential", change.Name)
		}
	default:
		return fmt.Errorf("provider configuration change %q has invalid desired state %q", change.Name, change.Spec.DesiredState)
	}
	return nil
}

func (h *Handler) reconcileAuthProvider(ctx context.Context, client kclient.Client, change *v1.ProviderConfigurationChange, stagedSecrets map[string]string) error {
	var authProvider v1.AuthProvider
	if err := client.Get(ctx, kclient.ObjectKey{Namespace: change.Namespace, Name: change.Spec.ProviderName}, &authProvider); err != nil {
		return fmt.Errorf("get auth provider %q: %w", change.Spec.ProviderName, err)
	}

	if change.Spec.DesiredState == v1.ProviderDesiredStateConfigured {
		configuredProvider, err := h.dispatcher.GetConfiguredAuthProvider(ctx)
		if err != nil {
			return fmt.Errorf("get configured auth provider: %w", err)
		}
		if configuredProvider != "" && configuredProvider != authProvider.Name {
			return &authProviderConflictError{configuredProvider: configuredProvider}
		}
		if err := h.gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
			Context: authProvider.Name,
			Name:    authProvider.Name,
			Secrets: stagedSecrets,
		}); err != nil {
			return fmt.Errorf("promote credential for auth provider %q: %w", authProvider.Name, err)
		}
	} else {
		if err := h.deconfigureAuthProvider(ctx, client, authProvider); err != nil {
			return err
		}
	}

	if err := provider.SetAuthProviderConfiguredStatus(ctx, h.gatewayClient, h.licenseProvider, &authProvider); err != nil {
		return fmt.Errorf("recompute auth provider %q status: %w", authProvider.Name, err)
	}
	if err := client.Status().Update(ctx, &authProvider); err != nil {
		return fmt.Errorf("update auth provider %q status: %w", authProvider.Name, err)
	}
	return nil
}

func (h *Handler) deconfigureAuthProvider(ctx context.Context, client kclient.Client, authProvider v1.AuthProvider) error {
	var cleanup *v1.AuthProviderCleanup
	if authProvider.Spec.GroupIDPrefix != "" {
		var err error
		cleanup, err = ensureAuthProviderCleanup(ctx, client, authProvider)
		if err != nil {
			return err
		}
	}

	if err := deleteResolvedCredential(ctx, h.gatewayClient, []string{authProvider.Name, system.GenericAuthProviderCredentialContext}, authProvider.Name); err != nil {
		return fmt.Errorf("remove credential for auth provider %q: %w", authProvider.Name, err)
	}

	if authProvider.Name == localauth.ProviderName {
		if err := h.gatewayClient.DeleteAllLocalAuthSessions(ctx); err != nil {
			return fmt.Errorf("delete local auth sessions: %w", err)
		}
	}
	if err := dropAuthProviderSessionTables(authProvider, h.postgresDSN); err != nil {
		return err
	}

	if cleanup != nil {
		if err := markAuthProviderCleanupReady(ctx, client, cleanup.Name); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) reconcileModelProvider(ctx context.Context, client kclient.Client, change *v1.ProviderConfigurationChange, stagedSecrets map[string]string) error {
	var modelProvider v1.ModelProvider
	if err := client.Get(ctx, kclient.ObjectKey{Namespace: change.Namespace, Name: change.Spec.ProviderName}, &modelProvider); err != nil {
		return fmt.Errorf("get model provider %q: %w", change.Spec.ProviderName, err)
	}

	if change.Spec.DesiredState == v1.ProviderDesiredStateConfigured {
		if err := h.gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
			Context: modelProvider.Name,
			Name:    modelProvider.Name,
			Secrets: stagedSecrets,
		}); err != nil {
			return fmt.Errorf("promote credential for model provider %q: %w", modelProvider.Name, err)
		}
	} else if err := deleteResolvedCredential(ctx, h.gatewayClient, []string{modelProvider.Name, system.GenericModelProviderCredentialContext}, modelProvider.Name); err != nil {
		return fmt.Errorf("remove credential for model provider %q: %w", modelProvider.Name, err)
	}

	modelProvider.Status.Error = ""
	modelProvider.Status.ModelsBackPopulated = nil
	if err := provider.SetModelProviderConfiguredStatus(ctx, h.gatewayClient, h.licenseProvider, &modelProvider); err != nil {
		return fmt.Errorf("recompute model provider %q status: %w", modelProvider.Name, err)
	}
	if change.Spec.DesiredState == v1.ProviderDesiredStateConfigured && modelProvider.Status.Configured {
		modelProvider.Status.ModelsBackPopulated = new(true)
		if err := provider.BackPopulateModels(ctx, client, h.dispatcher, &modelProvider); err != nil {
			return fmt.Errorf("refresh models for model provider %q: %w", modelProvider.Name, err)
		}
		if modelProvider.Status.Error != "" {
			modelProvider.Status.ModelsBackPopulated = new(false)
		}
	}
	if err := client.Status().Update(ctx, &modelProvider); err != nil {
		return fmt.Errorf("update model provider %q status: %w", modelProvider.Name, err)
	}
	return nil
}

func deleteResolvedCredential(ctx context.Context, gatewayClient *gateway.Client, contexts []string, credentialName string) error {
	credential, err := gatewayClient.RevealCredential(ctx, contexts, credentialName)
	if err != nil {
		if errors.As(err, &gateway.CredentialNotFoundError{}) {
			return nil
		}
		return err
	}
	_, err = gatewayClient.DeleteCredential(ctx, credential.Context, credentialName)
	return err
}

func ensureAuthProviderCleanup(ctx context.Context, client kclient.Client, authProvider v1.AuthProvider) (*v1.AuthProviderCleanup, error) {
	cleanupName := name.SafeConcatName(system.AuthProviderCleanupPrefix, authProvider.Name)
	var cleanup v1.AuthProviderCleanup
	if err := client.Get(ctx, kclient.ObjectKey{Namespace: authProvider.Namespace, Name: cleanupName}, &cleanup); err == nil {
		if cleanup.Spec.AuthProviderName != authProvider.Name || cleanup.Spec.GroupIDPrefix != authProvider.Spec.GroupIDPrefix {
			return nil, fmt.Errorf("auth provider cleanup %q does not match auth provider %q", cleanupName, authProvider.Name)
		}
		return &cleanup, nil
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("get auth provider cleanup %q: %w", cleanupName, err)
	}

	cleanup = v1.AuthProviderCleanup{
		Name:      cleanupName,
		Namespace: authProvider.Namespace,
		Spec: v1.AuthProviderCleanupSpec{
			AuthProviderName: authProvider.Name,
			GroupIDPrefix:    authProvider.Spec.GroupIDPrefix,
		},
	}
	if err := client.Create(ctx, &cleanup); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return ensureAuthProviderCleanup(ctx, client, authProvider)
		}
		return nil, fmt.Errorf("persist cleanup intent for auth provider %q: %w", authProvider.Name, err)
	}
	return &cleanup, nil
}

func markAuthProviderCleanupReady(ctx context.Context, client kclient.Client, cleanupName string) error {
	var cleanup v1.AuthProviderCleanup
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: cleanupName}, &cleanup); err != nil {
			return err
		}
		if cleanup.Spec.Ready {
			return nil
		}
		cleanup.Spec.Ready = true
		return client.Update(ctx, &cleanup)
	})
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("mark auth provider cleanup %q ready: %w", cleanupName, err)
	}
	return nil
}

func dropAuthProviderSessionTables(authProvider v1.AuthProvider, postgresDSN string) error {
	if postgresDSN == "" || authProvider.Spec.PostgresTablePrefix == "" {
		return nil
	}
	db, err := gorm.Open(postgres.Open(postgresDSN), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return fmt.Errorf("connect to postgres for auth provider cleanup: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get postgres connection for auth provider cleanup: %w", err)
	}
	defer sqlDB.Close()

	tablePrefix := authProvider.Spec.PostgresTablePrefix
	if err := db.Exec("DROP TABLE IF EXISTS " + tablePrefix + "sessions;").Error; err != nil {
		return fmt.Errorf("drop auth provider sessions table: %w", err)
	}
	if err := db.Exec("DROP TABLE IF EXISTS " + tablePrefix + "session_locks;").Error; err != nil {
		return fmt.Errorf("drop auth provider session locks table: %w", err)
	}
	return nil
}

func EnsureDaemonSync(ctx context.Context, client kclient.Client) error {
	sync := &v1.ProviderSync{
		Name:      system.ProviderSyncName,
		Namespace: system.DefaultNamespace,
	}
	if err := client.Create(ctx, sync); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("ensure provider daemon sync: %w", err)
	}
	return nil
}

// CleanupOrphanedStagedCredentials deletes old staged credentials that are not
// referenced by an existing ProviderConfigurationChange.
func CleanupOrphanedStagedCredentials(ctx context.Context, client kclient.Client, gatewayClient *gateway.Client, now time.Time, gracePeriod time.Duration) error {
	var changes v1.ProviderConfigurationChangeList
	if err := client.List(ctx, &changes); err != nil {
		return fmt.Errorf("list provider configuration changes: %w", err)
	}

	referencedCredentials := make(map[string]struct{}, len(changes.Items))
	for _, change := range changes.Items {
		if change.Spec.StagedCredentialName != "" {
			referencedCredentials[change.Spec.StagedCredentialName] = struct{}{}
		}
	}

	credentials, err := gatewayClient.ListCredentials(ctx, gateway.ListCredentialsOptions{
		CredentialContexts: []string{system.StagedProviderCredentialContext},
	})
	if err != nil {
		return fmt.Errorf("list staged provider credentials: %w", err)
	}

	cutoff := now.Add(-gracePeriod)
	var cleanupErrors []error
	for _, credential := range credentials {
		if _, ok := referencedCredentials[credential.Name]; ok || credential.CreatedAt.After(cutoff) {
			continue
		}
		if _, err := gatewayClient.DeleteCredential(ctx, system.StagedProviderCredentialContext, credential.Name); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("delete orphaned staged provider credential %q: %w", credential.Name, err))
		}
	}
	return errors.Join(cleanupErrors...)
}

func (h *Handler) advanceDaemonSync(ctx context.Context, client kclient.Client, change *v1.ProviderConfigurationChange) error {
	if err := EnsureDaemonSync(ctx, client); err != nil {
		return err
	}
	revisionKey := providerDaemonRevisionKey(change.Spec.ProviderType, change.Namespace, change.Spec.ProviderName)
	var daemonSync v1.ProviderSync
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.ProviderSyncName}, &daemonSync); err != nil {
			// EnsureDaemonSync above created the singleton if it was missing, so
			// anything failing here is worth requeuing the whole change for.
			return err
		}
		if daemonSync.Spec.Revisions == nil {
			daemonSync.Spec.Revisions = make(map[string]v1.ProviderRevision)
		}
		revision := daemonSync.Spec.Revisions[revisionKey]
		revision.ProviderType = change.Spec.ProviderType
		revision.ProviderNamespace = change.Namespace
		revision.ProviderName = change.Spec.ProviderName
		revision.Revision++
		daemonSync.Spec.Revisions[revisionKey] = revision
		return client.Update(ctx, &daemonSync)
	})
	if err != nil {
		return fmt.Errorf("advance provider daemon sync: %w", err)
	}
	return nil
}

func providerDaemonRevisionKey(providerType v1.ProviderType, namespace, providerName string) string {
	return fmt.Sprintf("%s/%s/%s", providerType, namespace, providerName)
}
