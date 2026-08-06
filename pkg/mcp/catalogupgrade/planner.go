package catalogupgrade

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcp/catalogversion"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	obottunnel "github.com/obot-platform/obot/pkg/tunnel"
	"github.com/obot-platform/obot/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const reservationAnnotation = "obot.obot.ai/catalog-upgrade-reservation"

type CredentialStore interface {
	RevealCredential(context.Context, []string, string) (gatewaytypes.Credential, error)
	UpsertCredential(context.Context, gatewaytypes.Credential) error
	DeleteCredential(context.Context, string, string) (bool, error)
}

type ShutdownFunc func(context.Context, string) error

type Planner struct {
	client            client.Client
	credentials       CredentialStore
	shutdown          ShutdownFunc
	secretBindingTest func(context.Context, types.MCPServerManifest) error
	validationOptions mcp.ValidationOptions
}

func New(storage client.Client, credentials CredentialStore, shutdown ShutdownFunc, secretBindingTest func(context.Context, types.MCPServerManifest) error, validationOptions mcp.ValidationOptions) *Planner {
	return &Planner{
		client: storage, credentials: credentials, shutdown: shutdown,
		secretBindingTest: secretBindingTest, validationOptions: validationOptions,
	}
}

type internalPlan struct {
	public           types.CatalogUpgradePlan
	server           v1.MCPServer
	target           catalogversion.Resolved
	reusable         map[string]string
	credential       gatewaytypes.Credential
	credentialExists bool
}

func (p *Planner) Plan(ctx context.Context, serverID string, targetVersion *int) (types.CatalogUpgradePlan, error) {
	plan, err := p.plan(ctx, serverID, targetVersion)
	return plan.public, err
}

func (p *Planner) plan(ctx context.Context, serverID string, targetVersion *int) (internalPlan, error) {
	var server v1.MCPServer
	if err := p.client.Get(ctx, client.ObjectKey{Name: serverID, Namespace: "default"}, &server); err != nil {
		return internalPlan{}, err
	}
	if server.Spec.MCPServerCatalogEntryName == "" {
		return internalPlan{}, fmt.Errorf("MCP server %s is not catalog-derived", serverID)
	}
	if server.Spec.CompositeName != "" {
		return internalPlan{}, fmt.Errorf("component MCP server %s must be upgraded through its composite", serverID)
	}

	var (
		target catalogversion.Resolved
		err    error
	)
	if targetVersion != nil {
		target, err = catalogversion.ResolveExact(ctx, p.client, server.Namespace, server.Spec.MCPServerCatalogEntryName, *targetVersion, true)
	} else if server.Spec.PinnedCatalogEntryVersion {
		target, err = catalogversion.ResolveExact(ctx, p.client, server.Namespace, server.Spec.MCPServerCatalogEntryName, server.Spec.MCPServerCatalogEntryVersion, false)
	} else {
		target, err = catalogversion.ResolveDefault(ctx, p.client, server.Namespace, server.Spec.MCPServerCatalogEntryName)
	}
	if err != nil {
		return internalPlan{}, err
	}

	targetManifest, convertErr := mcp.ServerManifestFromCatalogEntryManifest(false, true, target.Version.Spec.Manifest, server.Spec.Manifest)
	if convertErr == nil {
		preserveAdminBindings(&targetManifest, server.Spec.Manifest)
	}
	public := types.CatalogUpgradePlan{
		ServerID:        server.Name,
		CatalogEntryID:  server.Spec.MCPServerCatalogEntryName,
		SourceVersion:   server.Spec.MCPServerCatalogEntryVersion,
		TargetVersion:   target.Version.Spec.Version,
		CurrentManifest: server.Spec.Manifest,
		TargetManifest:  targetManifest,
		RuntimeChanged:  server.Spec.Manifest.Runtime != targetManifest.Runtime,
	}
	if convertErr != nil {
		public.ValidationFailures = append(public.ValidationFailures, convertErr.Error())
	}

	if public.RuntimeChanged {
		public.Warnings = append(public.Warnings, types.CatalogUpgradeWarning{Code: "runtime-transition", Message: fmt.Sprintf("runtime changes from %s to %s", server.Spec.Manifest.Runtime, targetManifest.Runtime)})
	}
	if server.Spec.Manifest.Runtime == types.RuntimeContainerized && targetManifest.Runtime != types.RuntimeContainerized {
		public.DestructiveStorageCleanup = true
		public.Warnings = append(public.Warnings, types.CatalogUpgradeWarning{Code: "storage-cleanup", Message: "stopping the current containerized runtime deletes its managed volume or PVC"})
	}

	if convertErr == nil {
		opts := p.validationOptions
		opts.AllowMissingURL = true
		if err := mcp.ValidateServerManifest(ctx, targetManifest, !server.Spec.IsSingleUser(), opts); err != nil {
			public.ValidationFailures = append(public.ValidationFailures, err.Error())
		}
		if err := obottunnel.ValidateServerTunnelReferences(ctx, p.client, targetManifest); err != nil {
			public.ValidationFailures = append(public.ValidationFailures, err.Error())
		}
		if p.secretBindingTest != nil {
			if err := p.secretBindingTest(ctx, targetManifest); err != nil {
				public.ValidationFailures = append(public.ValidationFailures, err.Error())
			}
		}
	}

	if source, err := catalogversion.ResolveExact(ctx, p.client, server.Namespace, server.Spec.MCPServerCatalogEntryName, server.Spec.MCPServerCatalogEntryVersion, false); err == nil &&
		source.Version.Spec.Manifest.ServerUserType != target.Version.Spec.Manifest.ServerUserType {
		public.ValidationFailures = append(public.ValidationFailures, "server user type changes cannot be applied in place")
	}

	credential, credentialExists, err := p.revealCredential(ctx, server)
	if err != nil {
		return internalPlan{}, err
	}
	reusable := reusableConfiguration(server.Spec.Manifest, targetManifest, credential.Secrets)
	for key := range reusable {
		public.ReusableConfiguration = append(public.ReusableConfiguration, key)
	}
	slices.Sort(public.ReusableConfiguration)
	public.MissingRequiredEnvVars, public.MissingRequiredHeaders = missingConfiguration(targetManifest, reusable)
	if !server.Spec.IsSingleUser() {
		missingByInstance, affectedUsers, err := p.multiUserConfigurationImpact(ctx, server, targetManifest.MultiUserConfig)
		if err != nil {
			return internalPlan{}, err
		}
		public.MissingInstanceConfiguration = missingByInstance
		public.AffectedUsers = affectedUsers
		if len(missingByInstance) > 0 {
			public.Warnings = append(public.Warnings, types.CatalogUpgradeWarning{
				Code: "user-reconfiguration", Message: fmt.Sprintf("%d user instances require additional configuration", len(missingByInstance)),
			})
		}
	}
	public.MissingURL = targetNeedsURL(target.Version.Spec.Manifest, targetManifest)
	public.OAuthReauthorizationRequired = oauthReauthorizationRequired(server, targetManifest)
	if target.Version.Spec.Manifest.Runtime == types.RuntimeRemote && target.Version.Spec.Manifest.RemoteConfig != nil &&
		target.Version.Spec.Manifest.RemoteConfig.StaticOAuthRequired && !target.Entry.Status.OAuthCredentialConfigured {
		public.ValidationFailures = append(public.ValidationFailures, "required static OAuth credentials are not configured")
	}
	public.CanApply = len(public.ValidationFailures) == 0 && len(public.MissingRequiredEnvVars) == 0 &&
		len(public.MissingRequiredHeaders) == 0 && !public.MissingURL && !public.OAuthReauthorizationRequired
	public.ID = utils.Digest(struct {
		ServerSpec    string
		TargetSpec    string
		TargetVersion int
	}{
		ServerSpec:    utils.Digest(server.Spec),
		TargetSpec:    utils.Digest(target.Version.Spec),
		TargetVersion: target.Version.Spec.Version,
	})

	return internalPlan{
		public: public, server: server, target: target, reusable: reusable,
		credential: credential, credentialExists: credentialExists,
	}, nil
}

func (p *Planner) multiUserConfigurationImpact(ctx context.Context, server v1.MCPServer, target *types.MultiUserConfig) (map[string][]string, int, error) {
	var instances v1.MCPServerInstanceList
	if err := p.client.List(ctx, &instances, client.InNamespace(server.Namespace)); err != nil {
		return nil, 0, err
	}
	missingByInstance := make(map[string][]string)
	users := make(map[string]struct{})
	for _, instance := range instances.Items {
		if instance.Spec.MCPServerName != server.Name || instance.Spec.Template || instance.Spec.CompositeName != "" {
			continue
		}
		users[instance.Spec.UserID] = struct{}{}
		if target == nil || len(target.UserDefinedHeaders) == 0 {
			continue
		}
		values := map[string]string{}
		if p.credentials != nil {
			credential, err := p.credentials.RevealCredential(ctx, []string{fmt.Sprintf("%s-%s", instance.Spec.UserID, instance.Name)}, instance.Name)
			if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
				return nil, 0, err
			}
			values = credential.Secrets
		}
		current := make(map[string]types.MCPHeader)
		if instance.Spec.MultiUserConfig != nil {
			for _, header := range instance.Spec.MultiUserConfig.UserDefinedHeaders {
				current[header.Key] = header
			}
		}
		for _, header := range target.UserDefinedHeaders {
			old, exists := current[header.Key]
			if header.Required && (!exists || !compatibleHeader(old, header) || values[header.Key] == "") {
				missingByInstance[instance.Name] = append(missingByInstance[instance.Name], header.Key)
			}
		}
		slices.Sort(missingByInstance[instance.Name])
	}
	if len(missingByInstance) == 0 {
		missingByInstance = nil
	}
	return missingByInstance, len(users), nil
}

func (p *Planner) Apply(ctx context.Context, serverID string, request types.CatalogUpgradeApplyRequest) (types.CatalogUpgradeResult, error) {
	plan, err := p.plan(ctx, serverID, nil)
	if err != nil {
		return types.CatalogUpgradeResult{}, err
	}
	if request.PlanID == "" || request.PlanID != plan.public.ID {
		return types.CatalogUpgradeResult{}, fmt.Errorf("upgrade plan is stale; preview the upgrade again")
	}

	configuration := plan.reusable
	for key, value := range request.Configuration {
		configuration[key] = value
	}
	missingURL := plan.public.MissingURL
	if request.URL != "" {
		if plan.public.TargetManifest.Runtime != types.RuntimeRemote || plan.public.TargetManifest.RemoteConfig == nil {
			return types.CatalogUpgradeResult{}, fmt.Errorf("target server does not accept a configured URL")
		}
		configuredURL := request.URL
		if !strings.HasPrefix(configuredURL, "http://") && !strings.HasPrefix(configuredURL, "https://") {
			configuredURL = "https://" + configuredURL
		}
		if remote := plan.target.Version.Spec.Manifest.RemoteConfig; remote != nil && remote.Hostname != "" {
			if err := types.ValidateURLHostname(configuredURL, remote.Hostname); err != nil {
				return types.CatalogUpgradeResult{}, fmt.Errorf("configured URL is incompatible with target: %w", err)
			}
		}
		plan.public.TargetManifest.RemoteConfig.URL = configuredURL
		missingURL = false
	}
	missingEnv, missingHeaders := missingConfiguration(plan.public.TargetManifest, configuration)
	if len(plan.public.ValidationFailures) > 0 || len(missingEnv) > 0 || len(missingHeaders) > 0 || missingURL || plan.public.OAuthReauthorizationRequired && !request.ConfirmOAuthReauthorization {
		return types.CatalogUpgradeResult{}, fmt.Errorf("upgrade prerequisites are not satisfied")
	}
	opts := p.validationOptions
	opts.AllowMissingURL = false
	if err := mcp.ValidateServerManifest(ctx, plan.public.TargetManifest, !plan.server.Spec.IsSingleUser(), opts); err != nil {
		return types.CatalogUpgradeResult{}, fmt.Errorf("configured target is invalid: %w", err)
	}

	reservationID := plan.public.ID + "-" + rand.Text()
	reserved, err := p.reserve(ctx, plan.server, reservationID)
	if err != nil {
		return types.CatalogUpgradeResult{}, err
	}
	plan.server = reserved
	releaseReservation := func() error {
		return p.releaseReservation(ctx, plan.server.Name, reservationID)
	}
	if err := p.validateTarget(ctx, plan); err != nil {
		return types.CatalogUpgradeResult{}, errors.Join(err, releaseReservation())
	}

	credentialContext := credentialContext(plan.server)
	credentialsWritten := false
	if p.credentials != nil && (plan.credentialExists || len(configuration) > 0) {
		if err := p.credentials.UpsertCredential(ctx, gatewaytypes.Credential{Context: credentialContext, Name: plan.server.Name, Secrets: configuration}); err != nil {
			rollbackErr := errors.Join(p.restoreCredential(ctx, plan), releaseReservation())
			return types.CatalogUpgradeResult{}, errors.Join(fmt.Errorf("failed to persist upgrade configuration: %w", err), rollbackErr)
		}
		credentialsWritten = true
	}
	if p.shutdown != nil {
		if err := p.shutdown(ctx, plan.server.Name); err != nil {
			var rollbackErr error
			if credentialsWritten {
				rollbackErr = p.restoreCredential(ctx, plan)
			}
			return types.CatalogUpgradeResult{}, errors.Join(err, rollbackErr, releaseReservation())
		}
	}

	if err := p.finalize(ctx, plan, reservationID); err != nil {
		var rollbackErr error
		if credentialsWritten {
			rollbackErr = p.restoreCredential(ctx, plan)
		}
		return types.CatalogUpgradeResult{}, errors.Join(err, rollbackErr, releaseReservation())
	}

	return types.CatalogUpgradeResult{
		ServerID: serverID, SourceVersion: plan.public.SourceVersion,
		TargetVersion: plan.public.TargetVersion, Applied: true,
	}, nil
}

func (p *Planner) reserve(ctx context.Context, source v1.MCPServer, reservationID string) (v1.MCPServer, error) {
	var reserved v1.MCPServer
	sourceSpec := utils.Digest(source.Spec)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest v1.MCPServer
		if err := p.client.Get(ctx, client.ObjectKeyFromObject(&source), &latest); err != nil {
			return err
		}
		if utils.Digest(latest.Spec) != sourceSpec {
			return fmt.Errorf("upgrade plan is stale: MCP server specification changed")
		}
		if current := latest.Annotations[reservationAnnotation]; current != "" {
			return fmt.Errorf("another catalog upgrade is already in progress")
		}
		if latest.Annotations == nil {
			latest.Annotations = make(map[string]string, 1)
		}
		latest.Annotations[reservationAnnotation] = reservationID
		if err := p.client.Update(ctx, &latest); err != nil {
			return err
		}
		reserved = latest
		return nil
	})
	if err != nil {
		return v1.MCPServer{}, err
	}
	return reserved, nil
}

func (p *Planner) finalize(ctx context.Context, plan internalPlan, reservationID string) error {
	sourceSpec := utils.Digest(plan.server.Spec)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := p.validateTarget(ctx, plan); err != nil {
			return err
		}
		var latest v1.MCPServer
		if err := p.client.Get(ctx, client.ObjectKeyFromObject(&plan.server), &latest); err != nil {
			return err
		}
		if latest.Annotations[reservationAnnotation] != reservationID || utils.Digest(latest.Spec) != sourceSpec {
			return fmt.Errorf("upgrade plan is stale: MCP server specification changed")
		}
		latest.Spec.Manifest = plan.public.TargetManifest
		latest.Spec.UnsupportedTools = plan.target.Version.Spec.UnsupportedTools
		latest.Spec.MCPServerCatalogEntryVersion = plan.target.Version.Spec.Version
		latest.Spec.NeedsURL = false
		latest.Spec.PreviousURL = ""
		delete(latest.Annotations, reservationAnnotation)
		return p.client.Update(ctx, &latest)
	})
}

func (p *Planner) validateTarget(ctx context.Context, plan internalPlan) error {
	var (
		current catalogversion.Resolved
		err     error
	)
	if plan.server.Spec.PinnedCatalogEntryVersion {
		current, err = catalogversion.ResolveExact(ctx, p.client, plan.server.Namespace, plan.server.Spec.MCPServerCatalogEntryName, plan.server.Spec.MCPServerCatalogEntryVersion, false)
	} else {
		current, err = catalogversion.ResolveDefault(ctx, p.client, plan.server.Namespace, plan.server.Spec.MCPServerCatalogEntryName)
	}
	if err != nil || current.Version.Spec.Version != plan.target.Version.Spec.Version || utils.Digest(current.Version.Spec) != utils.Digest(plan.target.Version.Spec) {
		return fmt.Errorf("upgrade plan is stale: target catalog version changed")
	}
	return nil
}

func (p *Planner) releaseReservation(ctx context.Context, serverName, reservationID string) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var server v1.MCPServer
		if err := p.client.Get(ctx, client.ObjectKey{Name: serverName, Namespace: "default"}, &server); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		if server.Annotations[reservationAnnotation] != reservationID {
			return nil
		}
		delete(server.Annotations, reservationAnnotation)
		return p.client.Update(ctx, &server)
	})
}

func (p *Planner) restoreCredential(ctx context.Context, plan internalPlan) error {
	if p.credentials == nil {
		return nil
	}
	if plan.credentialExists {
		return p.credentials.UpsertCredential(ctx, plan.credential)
	}
	_, err := p.credentials.DeleteCredential(ctx, credentialContext(plan.server), plan.server.Name)
	return err
}

func (p *Planner) revealCredential(ctx context.Context, server v1.MCPServer) (gatewaytypes.Credential, bool, error) {
	if p.credentials == nil {
		return gatewaytypes.Credential{Secrets: map[string]string{}}, false, nil
	}
	credential, err := p.credentials.RevealCredential(ctx, []string{credentialContext(server)}, server.Name)
	if errors.As(err, &gateway.CredentialNotFoundError{}) {
		return gatewaytypes.Credential{Context: credentialContext(server), Name: server.Name, Secrets: map[string]string{}}, false, nil
	}
	return credential, err == nil, err
}

func credentialContext(server v1.MCPServer) string {
	switch {
	case server.Spec.MCPCatalogID != "":
		return fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name)
	case server.Spec.PowerUserWorkspaceID != "":
		return fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name)
	default:
		return fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)
	}
}

func reusableConfiguration(current, target types.MCPServerManifest, values map[string]string) map[string]string {
	reusable := make(map[string]string)
	currentEnv := make(map[string]types.MCPEnv, len(current.Env))
	for _, field := range current.Env {
		currentEnv[field.Key] = field
	}
	for _, field := range target.Env {
		old, ok := currentEnv[field.Key]
		if ok && compatibleEnv(old, field) && values[field.Key] != "" && field.Value == "" && field.SecretBinding == nil {
			reusable[field.Key] = values[field.Key]
		}
	}
	if target.RemoteConfig == nil {
		return reusable
	}
	currentHeaders := map[string]types.MCPHeader{}
	if current.RemoteConfig != nil {
		for _, field := range current.RemoteConfig.Headers {
			currentHeaders[field.Key] = field
		}
	}
	for _, field := range target.RemoteConfig.Headers {
		old, ok := currentHeaders[field.Key]
		if ok && compatibleHeader(old, field) && values[field.Key] != "" && field.Value == "" && field.SecretBinding == nil {
			reusable[field.Key] = values[field.Key]
		}
	}
	return reusable
}

func compatibleEnv(left, right types.MCPEnv) bool {
	return left.Key == right.Key && left.Sensitive == right.Sensitive && left.File == right.File &&
		left.DynamicFile == right.DynamicFile && compatibleBinding(left.SecretBinding, right.SecretBinding)
}

func compatibleHeader(left, right types.MCPHeader) bool {
	return left.Key == right.Key && left.Sensitive == right.Sensitive && left.Prefix == right.Prefix && compatibleBinding(left.SecretBinding, right.SecretBinding)
}

func compatibleBinding(left, right *types.MCPSecretBinding) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Name == right.Name && left.Key == right.Key
}

func missingConfiguration(manifest types.MCPServerManifest, values map[string]string) (env, headers []string) {
	for _, field := range manifest.Env {
		if field.Required && field.Value == "" && field.SecretBinding == nil && values[field.Key] == "" {
			env = append(env, field.Key)
		}
	}
	if manifest.RemoteConfig != nil {
		for _, field := range manifest.RemoteConfig.Headers {
			if field.Required && field.Value == "" && field.SecretBinding == nil && values[field.Key] == "" {
				headers = append(headers, field.Key)
			}
		}
	}
	slices.Sort(env)
	slices.Sort(headers)
	return env, headers
}

func targetNeedsURL(catalog types.MCPServerCatalogEntryManifest, server types.MCPServerManifest) bool {
	return catalog.Runtime == types.RuntimeRemote && catalog.RemoteConfig != nil &&
		(catalog.RemoteConfig.Hostname != "" || catalog.RemoteConfig.URLTemplate != "") &&
		(server.RemoteConfig == nil || server.RemoteConfig.URL == "")
}

func oauthReauthorizationRequired(server v1.MCPServer, target types.MCPServerManifest) bool {
	if !server.Status.UserHasAuthenticated {
		return false
	}
	if server.Spec.Manifest.Runtime != types.RuntimeRemote || target.Runtime != types.RuntimeRemote ||
		server.Spec.Manifest.RemoteConfig == nil || target.RemoteConfig == nil {
		return true
	}
	currentURL := strings.TrimSuffix(server.Spec.Manifest.RemoteConfig.URL, "/")
	targetURL := strings.TrimSuffix(target.RemoteConfig.URL, "/")
	return currentURL == "" || targetURL == "" || currentURL != targetURL ||
		server.Spec.Manifest.RemoteConfig.StaticOAuthRequired != target.RemoteConfig.StaticOAuthRequired
}

func preserveAdminBindings(target *types.MCPServerManifest, current types.MCPServerManifest) {
	currentEnv := make(map[string]*types.MCPSecretBinding)
	for _, field := range current.Env {
		if field.SecretBinding != nil && field.SecretBinding.AdminAdded {
			currentEnv[field.Key] = field.SecretBinding
		}
	}
	for i := range target.Env {
		if target.Env[i].SecretBinding == nil && currentEnv[target.Env[i].Key] != nil {
			target.Env[i].SecretBinding = currentEnv[target.Env[i].Key]
		}
	}
	if target.RemoteConfig == nil || current.RemoteConfig == nil {
		return
	}
	currentHeaders := make(map[string]*types.MCPSecretBinding)
	for _, field := range current.RemoteConfig.Headers {
		if field.SecretBinding != nil && field.SecretBinding.AdminAdded {
			currentHeaders[field.Key] = field.SecretBinding
		}
	}
	for i := range target.RemoteConfig.Headers {
		if target.RemoteConfig.Headers[i].SecretBinding == nil && currentHeaders[target.RemoteConfig.Headers[i].Key] != nil {
			target.RemoteConfig.Headers[i].SecretBinding = currentHeaders[target.RemoteConfig.Headers[i].Key]
		}
	}
}
