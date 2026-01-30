package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/obot-platform/nah"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/data"
	"github.com/obot-platform/obot/pkg/controller/handlers/adminworkspace"
	"github.com/obot-platform/obot/pkg/controller/handlers/deployment"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpcatalog"
	"github.com/obot-platform/obot/pkg/controller/handlers/toolreference"
	"github.com/obot-platform/obot/pkg/services"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"

	// Enable logrus logging in nah
	_ "github.com/obot-platform/nah/pkg/logrus"
)

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

	if err := ensureK8sSettings(ctx, c.services.StorageClient, c.services.K8sSettingsFromHelm); err != nil {
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

	return nil
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

func ensureK8sSettings(ctx context.Context, client kclient.Client, helmSettings *v1.K8sSettingsSpec) error {
	var k8sSettings v1.K8sSettings
	if err := client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.K8sSettingsName,
	}, &k8sSettings); apierrors.IsNotFound(err) {
		// Create default settings
		k8sSettings = v1.K8sSettings{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.K8sSettingsName,
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.K8sSettingsSpec{
				SetViaHelm: helmSettings != nil,
			},
		}

		// If Helm settings provided, use them
		if helmSettings != nil {
			k8sSettings.Spec.Affinity = helmSettings.Affinity
			k8sSettings.Spec.Tolerations = helmSettings.Tolerations
			k8sSettings.Spec.Resources = helmSettings.Resources
			k8sSettings.Spec.RuntimeClassName = helmSettings.RuntimeClassName
		}

		return client.Create(ctx, &k8sSettings)
	} else if err != nil {
		return err
	}

	// If Helm settings provided, update them
	if helmSettings != nil {
		k8sSettings.Spec.SetViaHelm = true
		k8sSettings.Spec.Affinity = helmSettings.Affinity
		k8sSettings.Spec.Tolerations = helmSettings.Tolerations
		k8sSettings.Spec.Resources = helmSettings.Resources
		k8sSettings.Spec.RuntimeClassName = helmSettings.RuntimeClassName
		return client.Update(ctx, &k8sSettings)
	} else if k8sSettings.Spec.SetViaHelm {
		// Clear the settings if they were set by Helm, but are now blank.
		k8sSettings.Spec = v1.K8sSettingsSpec{}
		return client.Update(ctx, &k8sSettings)
	}

	return nil
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
