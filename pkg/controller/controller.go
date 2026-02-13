package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/obot-platform/nah"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/controller/data"
	"github.com/obot-platform/obot/pkg/controller/handlers/adminworkspace"
	"github.com/obot-platform/obot/pkg/controller/handlers/deployment"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpcatalog"
	"github.com/obot-platform/obot/pkg/controller/handlers/toolreference"
	"github.com/obot-platform/obot/pkg/services"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"

	// Enable logrus logging in nah
	_ "github.com/obot-platform/nah/pkg/logrus"
)

var log = logger.Package()

type Controller struct {
	router                *router.Router
	localK8sRouter        *router.Router
	services              *services.Services
	toolRefHandler        *toolreference.Handler
	mcpCatalogHandler     *mcpcatalog.Handler
	adminWorkspaceHandler *adminworkspace.Handler
}

func New(services *services.Services) (*Controller, error) {
	c := &Controller{
		router:   services.Router,
		services: services,
	}

	// Create local Kubernetes router if MCP is enabled and config is available
	var err error
	if services.LocalK8sConfig != nil {
		c.localK8sRouter, err = c.createLocalK8sRouter()
		if err != nil {
			// Log warning but don't fail - MCP deployment monitoring is optional
			return nil, fmt.Errorf("failed to create local Kubernetes router: %w", err)
		}
	}

	c.setupRoutes()
	c.setupLocalK8sRoutes()

	services.Router.PosStart(c.PostStart)

	return c, nil
}

func (c *Controller) PreStart(ctx context.Context) error {
	if err := data.Data(ctx, c.services.StorageClient, c.services.AgentsDir); err != nil {
		return fmt.Errorf("failed to apply data: %w", err)
	}

	if err := ensureDefaultUserRoleSetting(ctx, c.services.StorageClient); err != nil {
		return fmt.Errorf("failed to ensure default user role setting: %w", err)
	}

	if err := ensureK8sSettings(ctx, c.services.StorageClient, c.services.PodSchedulingSettingsFromHelm, c.services.PSASettingsFromHelm); err != nil {
		return fmt.Errorf("failed to ensure K8s settings: %w", err)
	}

	if err := ensureAppPreferences(ctx, c.services.StorageClient); err != nil {
		return fmt.Errorf("failed to ensure app preferences: %w", err)
	}

	if err := addCatalogIDToAccessControlRules(ctx, c.services.StorageClient); err != nil {
		return fmt.Errorf("failed to add catalog ID to access control rules: %w", err)
	}

	// Ensure PowerUserWorkspaces exist for all admin users on startup
	if err := c.adminWorkspaceHandler.EnsureAllAdminAndOwnerWorkspaces(ctx, c.services.StorageClient, system.DefaultNamespace); err != nil {
		return fmt.Errorf("failed to ensure admin workspaces: %w", err)
	}

	if c.services.NanobotIntegration {
		if err := c.ensureObotMCPServer(ctx); err != nil {
			return fmt.Errorf("failed to ensure obot MCP server: %w", err)
		}
	}

	return nil
}

func (c *Controller) ensureObotMCPServer(ctx context.Context) error {
	internalURL := c.services.MCPLoader.TransformObotHostname(c.services.ServerURL)
	image := c.services.MCPServerSearchImage

	var existing v1.SystemMCPServer
	err := c.services.StorageClient.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.ObotMCPServerName,
	}, &existing)
	if err == nil {
		// Reconcile all critical fields to ensure the server is correctly configured
		var needsUpdate bool

		if !existing.Spec.Manifest.Enabled {
			existing.Spec.Manifest.Enabled = true
			needsUpdate = true
		}

		if existing.Spec.Manifest.Runtime != types.RuntimeContainerized {
			existing.Spec.Manifest.Runtime = types.RuntimeContainerized
			needsUpdate = true
		}

		expectedConfig := &types.ContainerizedRuntimeConfig{
			Image: image,
			Port:  8080,
			Path:  "/mcp",
		}
		if existing.Spec.Manifest.ContainerizedConfig == nil {
			existing.Spec.Manifest.ContainerizedConfig = expectedConfig
			needsUpdate = true
		} else {
			if existing.Spec.Manifest.ContainerizedConfig.Image != image {
				existing.Spec.Manifest.ContainerizedConfig.Image = image
				needsUpdate = true
			}
			if existing.Spec.Manifest.ContainerizedConfig.Port != 8080 {
				existing.Spec.Manifest.ContainerizedConfig.Port = 8080
				needsUpdate = true
			}
			if existing.Spec.Manifest.ContainerizedConfig.Path != "/mcp" {
				existing.Spec.Manifest.ContainerizedConfig.Path = "/mcp"
				needsUpdate = true
			}
		}

		// Check OBOT_URL env var
		foundOBOTURLEntry := false
		for i, env := range existing.Spec.Manifest.Env {
			if env.Key == "OBOT_URL" {
				foundOBOTURLEntry = true
				if env.Value != internalURL {
					existing.Spec.Manifest.Env[i].Value = internalURL
					needsUpdate = true
				}
			}
		}
		if !foundOBOTURLEntry {
			existing.Spec.Manifest.Env = append(existing.Spec.Manifest.Env, types.MCPEnv{
				MCPHeader: types.MCPHeader{
					Name:     "OBOT_URL",
					Key:      "OBOT_URL",
					Required: true,
					Value:    internalURL,
				},
			})
			needsUpdate = true
		}

		if needsUpdate {
			log.Infof("Updating obot MCP server (image=%s)", image)
			return c.services.StorageClient.Update(ctx, &existing)
		}
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return err
	}

	// Create the SystemMCPServer
	log.Infof("Creating obot MCP server (image=%s)", image)
	server := &v1.SystemMCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:       system.ObotMCPServerName,
			Namespace:  system.DefaultNamespace,
			Finalizers: []string{v1.SystemMCPServerFinalizer},
		},
		Spec: v1.SystemMCPServerSpec{
			Manifest: types.SystemMCPServerManifest{
				Name:             "Obot MCP Server",
				ShortDescription: "MCP server for discovering and searching available MCP servers",
				Enabled:          true,
				Runtime:          types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: image,
					Port:  8080,
					Path:  "/mcp",
				},
				Env: []types.MCPEnv{
					{
						MCPHeader: types.MCPHeader{
							Name:     "OBOT_URL",
							Key:      "OBOT_URL",
							Required: true,
							Value:    internalURL,
						},
					},
				},
			},
		},
	}

	return c.services.StorageClient.Create(ctx, server)
}

func (c *Controller) PostStart(ctx context.Context, client kclient.Client) {
	go c.toolRefHandler.PollRegistries(ctx, client)
	var err error
	for range 3 {
		err = c.toolRefHandler.EnsureOpenAIEnvCredentialAndDefaults(ctx, client)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond) // wait a bit before retrying
	}
	if err != nil {
		panic(fmt.Errorf("failed to ensure openai env credential and defaults: %w", err))
	}

	if err := c.services.PersistentTokenServer.EnsureJWK(ctx); err != nil {
		panic(fmt.Errorf("failed to ensure JWK: %w", err))
	}

	if err = c.toolRefHandler.EnsureAnthropicCredentialAndDefaults(ctx, client); err != nil {
		panic(fmt.Errorf("failed to ensure anthropic credential and defaults: %w", err))
	}

	if err := c.mcpCatalogHandler.SetUpDefaultMCPCatalog(ctx, client); err != nil {
		panic(fmt.Errorf("failed to set up default mcp catalog: %w", err))
	}

	// Re-trigger all MCPServerCatalogEntries after startup to ensure MCPServers
	// that were reconciled before their catalog entries get notified of any pending updates.
	// This fixes a race condition where catalog entry changes might not trigger MCPServer
	// reconciliation if the server hadn't registered its watch yet.
	go c.retriggerCatalogEntries(ctx, client)
}

// retriggerCatalogEntries touches all MCPServerCatalogEntries to trigger their handlers,
// which in turn fires triggers to all MCPServers watching them. This ensures that any
// MCPServers that missed initial catalog entry change notifications get reconciled.
func (c *Controller) retriggerCatalogEntries(ctx context.Context, client kclient.Client) {
	// Wait a short period to allow initial reconciliation of MCPServers to complete.
	// This gives MCPServers time to register their watches on catalog entries.
	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Second):
	}

	var entries v1.MCPServerCatalogEntryList
	if err := client.List(ctx, &entries, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
	}); err != nil {
		log.Errorf("Failed to list MCPServerCatalogEntries for re-trigger: %v", err)
		return
	}

	log.Infof("Re-triggering %d MCPServerCatalogEntries to ensure MCPServer watches are established", len(entries.Items))

	for _, entry := range entries.Items {
		// Touch the entry's metadata to trigger reconciliation.
		// We use an annotation update to avoid modifying actual data.
		patch := kclient.MergeFrom(entry.DeepCopy())
		if entry.Annotations == nil {
			entry.Annotations = make(map[string]string)
		}
		entry.Annotations["obot.ai/startup-retrigger"] = time.Now().Format(time.RFC3339)

		if err := client.Patch(ctx, &entry, patch); err != nil {
			log.Warnf("Failed to re-trigger MCPServerCatalogEntry %s: %v", entry.Name, err)
			continue
		}
	}

	log.Infof("Completed re-triggering MCPServerCatalogEntries")
}

func (c *Controller) Start(ctx context.Context) error {
	if err := c.router.Start(ctx); err != nil {
		return fmt.Errorf("failed to start router: %w", err)
	}

	// Start the local Kubernetes router if it exists
	if c.localK8sRouter != nil {
		if err := c.localK8sRouter.Start(ctx); err != nil {
			return fmt.Errorf("failed to start local Kubernetes router: %w", err)
		}
	}

	return nil
}

func ensureDefaultUserRoleSetting(ctx context.Context, client kclient.Client) error {
	var defaultRoleSetting v1.UserDefaultRoleSetting
	if err := client.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.DefaultRoleSettingName}, &defaultRoleSetting); apierrors.IsNotFound(err) {
		defaultRoleSetting = v1.UserDefaultRoleSetting{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.DefaultRoleSettingName,
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.UserDefaultRoleSettingSpec{
				Role: types.RoleBasic,
			},
		}

		return client.Create(ctx, &defaultRoleSetting)
	} else if err != nil {
		return err
	}

	// If the role is 1, 2, 3, or 10, then this needs to be migrated to the new role system. Any other value means it was already migrated.
	switch defaultRoleSetting.Spec.Role {
	case 1:
		defaultRoleSetting.Spec.Role = types.RoleAdmin
	case 2:
		defaultRoleSetting.Spec.Role = types.RolePowerUserPlus
	case 3:
		defaultRoleSetting.Spec.Role = types.RolePowerUser
	case 10:
		defaultRoleSetting.Spec.Role = types.RoleBasic
	default:
		// Already migrated
		return nil
	}

	return client.Update(ctx, &defaultRoleSetting)
}

// ensureK8sSettings ensures the K8sSettings resource exists with proper configuration.
// podSchedulingSettings: affinity, tolerations, resources, runtimeClassName - can be managed via Helm OR UI.
//
//	If provided (non-nil), SetViaHelm=true and UI cannot modify these settings.
//
// psaSettings: Pod Security Admission settings - always sourced from Helm/environment.
//
//	These are always applied regardless of SetViaHelm flag and cannot be modified via UI.
func ensureK8sSettings(ctx context.Context, client kclient.Client, podSchedulingSettings *v1.K8sSettingsSpec, psaSettings *v1.PodSecurityAdmissionSettings) error {
	var k8sSettings v1.K8sSettings
	if err := client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.K8sSettingsName,
	}, &k8sSettings); apierrors.IsNotFound(err) {
		// Create default settings
		// SetViaHelm only applies to pod scheduling settings, not PSA
		k8sSettings = v1.K8sSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.K8sSettingsName,
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.K8sSettingsSpec{
				SetViaHelm: podSchedulingSettings != nil,
			},
		}

		// If pod scheduling settings provided via Helm, use them
		if podSchedulingSettings != nil {
			k8sSettings.Spec.Affinity = podSchedulingSettings.Affinity
			k8sSettings.Spec.Tolerations = podSchedulingSettings.Tolerations
			k8sSettings.Spec.Resources = podSchedulingSettings.Resources
			k8sSettings.Spec.RuntimeClassName = podSchedulingSettings.RuntimeClassName
			k8sSettings.Spec.StorageClassName = podSchedulingSettings.StorageClassName
			k8sSettings.Spec.NanobotWorkspaceSize = podSchedulingSettings.NanobotWorkspaceSize
		}

		// PSA settings are always applied from environment/Helm (independent of SetViaHelm)
		k8sSettings.Spec.PodSecurityAdmission = psaSettings

		return client.Create(ctx, &k8sSettings)
	} else if err != nil {
		return err
	}

	// Determine if we need to update
	needsUpdate := false

	// Handle pod scheduling settings from Helm
	if podSchedulingSettings != nil {
		// Pod scheduling settings provided via Helm - lock them
		if !k8sSettings.Spec.SetViaHelm ||
			!affinityEqual(k8sSettings.Spec.Affinity, podSchedulingSettings.Affinity) ||
			!tolerationsEqual(k8sSettings.Spec.Tolerations, podSchedulingSettings.Tolerations) ||
			!resourcesEqual(k8sSettings.Spec.Resources, podSchedulingSettings.Resources) ||
			!classNameEqual(k8sSettings.Spec.RuntimeClassName, podSchedulingSettings.RuntimeClassName) ||
			!classNameEqual(k8sSettings.Spec.StorageClassName, podSchedulingSettings.StorageClassName) ||
			!workspaceSizeEqual(k8sSettings.Spec.NanobotWorkspaceSize, podSchedulingSettings.NanobotWorkspaceSize) {
			k8sSettings.Spec.SetViaHelm = true
			k8sSettings.Spec.Affinity = podSchedulingSettings.Affinity
			k8sSettings.Spec.Tolerations = podSchedulingSettings.Tolerations
			k8sSettings.Spec.Resources = podSchedulingSettings.Resources
			k8sSettings.Spec.RuntimeClassName = podSchedulingSettings.RuntimeClassName
			k8sSettings.Spec.StorageClassName = podSchedulingSettings.StorageClassName
			k8sSettings.Spec.NanobotWorkspaceSize = podSchedulingSettings.NanobotWorkspaceSize
			needsUpdate = true
		}
	} else if k8sSettings.Spec.SetViaHelm {
		// Pod scheduling settings were previously set via Helm but are now blank
		// Clear them and allow UI management
		k8sSettings.Spec.SetViaHelm = false
		k8sSettings.Spec.Affinity = nil
		k8sSettings.Spec.Tolerations = nil
		k8sSettings.Spec.Resources = nil
		k8sSettings.Spec.RuntimeClassName = nil
		k8sSettings.Spec.StorageClassName = nil
		k8sSettings.Spec.NanobotWorkspaceSize = ""
		needsUpdate = true
	}

	// PSA settings are always sourced from environment/Helm (independent of SetViaHelm)
	if !psaSettingsEqual(k8sSettings.Spec.PodSecurityAdmission, psaSettings) {
		k8sSettings.Spec.PodSecurityAdmission = psaSettings
		needsUpdate = true
	}

	if needsUpdate {
		return client.Update(ctx, &k8sSettings)
	}

	return nil
}

// Helper functions for comparing settings
func affinityEqual(a, b *corev1.Affinity) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return equality.Semantic.DeepEqual(a, b)
}

func tolerationsEqual(a, b []corev1.Toleration) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return equality.Semantic.DeepEqual(a, b)
}

func resourcesEqual(a, b *corev1.ResourceRequirements) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return equality.Semantic.DeepEqual(a, b)
}

func classNameEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func workspaceSizeEqual(a, b string) bool {
	return a == b
}

func psaSettingsEqual(a, b *v1.PodSecurityAdmissionSettings) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Enabled == b.Enabled &&
		a.Enforce == b.Enforce &&
		a.EnforceVersion == b.EnforceVersion &&
		a.Audit == b.Audit &&
		a.AuditVersion == b.AuditVersion &&
		a.Warn == b.Warn &&
		a.WarnVersion == b.WarnVersion
}

func ensureAppPreferences(ctx context.Context, client kclient.Client) error {
	var appPrefs v1.AppPreferences
	err := client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.AppPreferencesName,
	}, &appPrefs)
	if apierrors.IsNotFound(err) {
		// Create default preferences
		appPrefs = v1.AppPreferences{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.AppPreferencesName,
				Namespace: system.DefaultNamespace,
			},
		}
		return kclient.IgnoreAlreadyExists(client.Create(ctx, &appPrefs))
	}
	return err
}

// createLocalK8sRouter creates a router for local Kubernetes resources
func (c *Controller) createLocalK8sRouter() (*router.Router, error) {
	// Create a scheme that includes the types we need to watch
	localScheme := scheme.Scheme
	if err := appsv1.AddToScheme(localScheme); err != nil {
		return nil, fmt.Errorf("failed to add appsv1 to scheme: %w", err)
	}

	localRouter, err := nah.NewRouter("obot-local-k8s", &nah.Options{
		RESTConfig:     c.services.LocalK8sConfig,
		Scheme:         localScheme,
		Namespace:      c.services.MCPServerNamespace,
		ElectionConfig: nil, // No leader election for local router
		HealthzPort:    -1,  // Disable healthz port
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create local Kubernetes router: %w", err)
	}

	return localRouter, nil
}

// setupLocalK8sRoutes sets up routes for the local Kubernetes router
func (c *Controller) setupLocalK8sRoutes() {
	if c.localK8sRouter == nil {
		return
	}

	deploymentHandler := deployment.New(c.services.MCPServerNamespace, c.services.Router.Backend())
	c.localK8sRouter.Type(&appsv1.Deployment{}).HandlerFunc(deploymentHandler.UpdateMCPServerStatus)
	c.localK8sRouter.Type(&appsv1.Deployment{}).HandlerFunc(deploymentHandler.CleanupOldIDs)
}
