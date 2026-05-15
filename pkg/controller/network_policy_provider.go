package controller

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"github.com/obot-platform/obot/pkg/services"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

const (
	networkPolicyProviderReleaseName    = "obot-network-policy-provider"
	networkPolicyProviderTokenMountPath = "/var/run/secrets/obot-network-policy-provider"
	networkPolicyProviderHelmTimeout    = 2 * time.Minute
)

type networkPolicyProviderInstallSpec struct {
	ReleaseName      string
	ReleaseNamespace string
	ChartRepoURL     string
	ChartName        string
	ChartVersion     string
	ChartPath        string
	Values           map[string]any
}

type networkPolicyProviderInstaller interface {
	InstallOrUpgrade(ctx context.Context, spec networkPolicyProviderInstallSpec) error
	Uninstall(releaseNamespace string) error
}

type helmNetworkPolicyProviderInstaller struct {
	restConfigFn func() (*rest.Config, error)
}

type helmLogCapture struct {
	lines []string
}

func (h *helmLogCapture) Debugf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	h.lines = append(h.lines, message)
	log.Debugf("%s", message)
}

func (h *helmLogCapture) Details() string {
	if len(h.lines) == 0 {
		return ""
	}
	return strings.Join(h.lines, "\n")
}

func helmActionError(err error, capture *helmLogCapture) error {
	if err == nil || capture == nil {
		return err
	}
	details := capture.Details()
	if details == "" {
		return err
	}
	return fmt.Errorf("%w\nHelm action log:\n%s", err, details)
}

func (c *Controller) reconcileNetworkPolicyProvider(ctx context.Context) error {
	if !c.services.MCPNetworkPolicyEnabled &&
		!mcp.IsKubernetesRuntimeBackend(c.services.MCPRuntimeBackend) {
		return nil
	}

	installer, err := c.networkPolicyProviderInstaller()
	if err != nil {
		return err
	}

	ns, err := c.runtimeNamespace()
	if err != nil {
		return err
	}

	if !c.services.MCPNetworkPolicyEnabled {
		return installer.Uninstall(ns)
	}

	spec, err := c.desiredNetworkPolicyProviderInstallSpec()
	if err != nil {
		return err
	}

	return installer.InstallOrUpgrade(ctx, spec)
}

func (c *Controller) networkPolicyProviderInstaller() (networkPolicyProviderInstaller, error) {
	if c.providerInstaller != nil {
		return c.providerInstaller, nil
	}

	c.providerInstaller = &helmNetworkPolicyProviderInstaller{
		restConfigFn: services.BuildLocalK8sConfig,
	}
	return c.providerInstaller, nil
}

func (c *Controller) desiredNetworkPolicyProviderInstallSpec() (networkPolicyProviderInstallSpec, error) {
	ns, err := c.runtimeNamespace()
	if err != nil {
		return networkPolicyProviderInstallSpec{}, err
	}

	releaseNamespace := ns
	storageURL, err := c.networkPolicyProviderStorageURL()
	if err != nil {
		return networkPolicyProviderInstallSpec{}, err
	}

	values, err := c.networkPolicyProviderValues(storageURL, releaseNamespace)
	if err != nil {
		return networkPolicyProviderInstallSpec{}, err
	}

	return networkPolicyProviderInstallSpec{
		ReleaseName:      networkPolicyProviderReleaseName,
		ReleaseNamespace: releaseNamespace,
		ChartRepoURL:     c.services.MCPNetworkPolicyProviderChartRepo,
		ChartName:        c.services.MCPNetworkPolicyProviderChartName,
		ChartVersion:     c.services.MCPNetworkPolicyProviderChartVersion,
		ChartPath:        c.services.MCPNetworkPolicyProviderChartPath,
		Values:           values,
	}, nil
}

func (c *Controller) networkPolicyProviderStorageURL() (string, error) {
	if c.services.ServiceName == "" || c.services.ServiceNamespace == "" || c.services.StorageListenPort == 0 {
		return "", fmt.Errorf("service name, service namespace, and storage listen port must be configured for the network policy provider")
	}

	return fmt.Sprintf("https://%s.%s.svc.%s:%d", c.services.ServiceName, c.services.ServiceNamespace, c.services.MCPClusterDomain, c.services.StorageListenPort), nil
}

func (c *Controller) networkPolicyProviderValues(storageURL, releaseNamespace string) (map[string]any, error) {
	if c.services.ServiceAccountName == "" {
		return nil, fmt.Errorf("service account name must be configured for the network policy provider")
	}

	values := map[string]any{
		"releaseNamespace":    releaseNamespace,
		"mcpRuntimeNamespace": c.services.MCPServerNamespace,
		"obotStorageURL":      storageURL,
		"secretName":          serviceaccounts.NetworkPolicySecretName,
		"obotStorageTokenFile": filepath.Join(
			networkPolicyProviderTokenMountPath,
			serviceaccounts.NetworkPolicySecretKey,
		),
		"obot": map[string]any{
			"serviceAccount": map[string]any{
				"name":      c.services.ServiceAccountName,
				"namespace": releaseNamespace,
			},
		},
	}

	if strings.TrimSpace(c.services.MCPNetworkPolicyProviderValues) == "" {
		return values, nil
	}

	var override map[string]any
	if err := yaml.Unmarshal([]byte(c.services.MCPNetworkPolicyProviderValues), &override); err != nil {
		return nil, fmt.Errorf("failed to parse OBOT_SERVER_MCPNETWORK_POLICY_PROVIDER_VALUES: %w", err)
	}

	return deepMergeMaps(values, override), nil
}

func deepMergeMaps(base, override map[string]any) map[string]any {
	if len(base) == 0 {
		return cloneMap(override)
	}

	merged := cloneMap(base)
	for key, value := range override {
		overrideMap, overrideIsMap := value.(map[string]any)
		baseMap, baseIsMap := merged[key].(map[string]any)
		if overrideIsMap && baseIsMap {
			merged[key] = deepMergeMaps(baseMap, overrideMap)
			continue
		}
		merged[key] = cloneValue(value)
	}
	return merged
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}

	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneValue(value)
	}
	return out
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = cloneValue(typed[i])
		}
		return out
	default:
		return typed
	}
}

func (h *helmNetworkPolicyProviderInstaller) InstallOrUpgrade(ctx context.Context, spec networkPolicyProviderInstallSpec) error {
	helmLogs := &helmLogCapture{}
	actionConfig, err := h.newActionConfigWithLog(spec.ReleaseNamespace, helmLogs.Debugf)
	if err != nil {
		return err
	}

	chartPath, cleanup, err := h.locateChart(spec)
	if err != nil {
		return err
	}
	defer cleanup()

	ch, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load network policy provider chart: %w", err)
	}

	get := action.NewGet(actionConfig)
	rel, err := get.Run(spec.ReleaseName)
	if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return fmt.Errorf("failed to inspect network policy provider release %q: %w", spec.ReleaseName, helmActionError(err, helmLogs))
	}

	if rel != nil {
		rel, err = recoverPendingRelease(actionConfig, rel)
		if err != nil {
			return fmt.Errorf("failed to recover pending network policy provider release %q: %w", spec.ReleaseName, helmActionError(err, helmLogs))
		}
	}

	if rel == nil {
		install := action.NewInstall(actionConfig)
		install.ReleaseName = spec.ReleaseName
		install.Namespace = spec.ReleaseNamespace
		install.Wait = true
		install.Atomic = true
		install.Timeout = networkPolicyProviderHelmTimeout

		if _, err := install.RunWithContext(ctx, ch, spec.Values); err != nil {
			return fmt.Errorf("failed to install network policy provider: %w", helmActionError(err, helmLogs))
		}
		return nil
	}

	upgrade := action.NewUpgrade(actionConfig)
	upgrade.Namespace = spec.ReleaseNamespace
	upgrade.ResetValues = true
	upgrade.Wait = true
	upgrade.Atomic = true
	upgrade.Timeout = networkPolicyProviderHelmTimeout

	if _, err := upgrade.RunWithContext(ctx, spec.ReleaseName, ch, spec.Values); err != nil {
		return fmt.Errorf("failed to upgrade network policy provider: %w", helmActionError(err, helmLogs))
	}

	return nil
}

// recoverPendingRelease clears any in-progress Helm operation left behind by a
// previous crash so the caller can safely retry install/upgrade. It returns
// the post-recovery release (nil when the pending install has been cleared and
// the caller should take the install path).
func recoverPendingRelease(actionConfig *action.Configuration, rel *release.Release) (*release.Release, error) {
	if rel == nil || rel.Info == nil {
		return rel, nil
	}

	switch rel.Info.Status {
	case release.StatusPendingInstall:
		uninstall := action.NewUninstall(actionConfig)
		uninstall.Wait = true
		uninstall.Timeout = networkPolicyProviderHelmTimeout
		if _, err := uninstall.Run(rel.Name); err != nil {
			return nil, fmt.Errorf("uninstalling pending-install release: %w", err)
		}
		return nil, nil
	case release.StatusPendingUpgrade, release.StatusPendingRollback:
		rollback := action.NewRollback(actionConfig)
		rollback.Wait = true
		rollback.Timeout = networkPolicyProviderHelmTimeout
		if err := rollback.Run(rel.Name); err != nil {
			return nil, fmt.Errorf("rolling back pending release: %w", err)
		}
		refreshed, err := action.NewGet(actionConfig).Run(rel.Name)
		if err != nil {
			return nil, fmt.Errorf("re-inspecting release after rollback: %w", err)
		}
		return refreshed, nil
	}
	return rel, nil
}

func (h *helmNetworkPolicyProviderInstaller) Uninstall(releaseNamespace string) error {
	helmLogs := &helmLogCapture{}
	actionConfig, err := h.newActionConfigWithLog(releaseNamespace, helmLogs.Debugf)
	if err != nil {
		return err
	}

	uninstall := action.NewUninstall(actionConfig)
	uninstall.Wait = true
	uninstall.Timeout = networkPolicyProviderHelmTimeout

	if _, err := uninstall.Run(networkPolicyProviderReleaseName); err != nil {
		if errors.Is(err, driver.ErrReleaseNotFound) || strings.Contains(err.Error(), "release: not found") {
			return nil
		}
		return fmt.Errorf("failed to uninstall network policy provider release %q: %w", networkPolicyProviderReleaseName, helmActionError(err, helmLogs))
	}

	return nil
}

func (h *helmNetworkPolicyProviderInstaller) locateChart(spec networkPolicyProviderInstallSpec) (string, func(), error) {
	if spec.ChartPath != "" {
		return spec.ChartPath, func() {}, nil
	}

	cacheDir, err := os.MkdirTemp("", "obot-helm-*")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create Helm cache directory: %w", err)
	}

	settings := cli.New()
	settings.RepositoryConfig = filepath.Join(cacheDir, "repositories.yaml")
	settings.RepositoryCache = filepath.Join(cacheDir, "repository")
	settings.RegistryConfig = filepath.Join(cacheDir, "registry.json")
	if err := os.MkdirAll(settings.RepositoryCache, 0o755); err != nil {
		_ = os.RemoveAll(cacheDir)
		return "", nil, fmt.Errorf("failed to initialize Helm repository cache: %w", err)
	}

	chartPathOptions := action.ChartPathOptions{
		RepoURL: spec.ChartRepoURL,
		Version: spec.ChartVersion,
	}
	chartPath, err := chartPathOptions.LocateChart(spec.ChartName, settings)
	if err != nil {
		_ = os.RemoveAll(cacheDir)
		return "", nil, fmt.Errorf("failed to locate network policy provider chart %q: %w", spec.ChartName, err)
	}

	return chartPath, func() {
		_ = os.RemoveAll(cacheDir)
	}, nil
}

func (h *helmNetworkPolicyProviderInstaller) newActionConfigWithLog(namespace string, debugLog action.DebugLog) (*action.Configuration, error) {
	restConfig, err := h.restConfigFn()
	if err != nil {
		return nil, err
	}

	flags := genericclioptions.NewConfigFlags(false)
	flags.Namespace = &namespace
	flags.WrapConfigFn = func(*rest.Config) *rest.Config {
		return rest.CopyConfig(restConfig)
	}

	actionConfig := &action.Configuration{}
	if err := actionConfig.Init(flags, namespace, "secret", debugLog); err != nil {
		return nil, fmt.Errorf("failed to initialize Helm action configuration: %w", err)
	}

	return actionConfig, nil
}
