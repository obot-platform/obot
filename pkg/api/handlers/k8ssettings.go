package handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const appContainerName = "obot"

type K8sSettingsHandler struct {
	mcpSessionManager   *mcp.SessionManager
	mcpRuntimeBackend   string
	serviceName         string
	serviceNamespace    string
	localK8sClient      client.Client
}

func NewK8sSettingsHandler(
	mcpSessionManager *mcp.SessionManager,
	mcpRuntimeBackend string,
	serviceName string,
	serviceNamespace string,
	localK8sClient client.Client,
) *K8sSettingsHandler {
	return &K8sSettingsHandler{
		mcpSessionManager: mcpSessionManager,
		mcpRuntimeBackend: mcpRuntimeBackend,
		serviceName:       serviceName,
		serviceNamespace:  serviceNamespace,
		localK8sClient:    localK8sClient,
	}
}

func (h *K8sSettingsHandler) GetApp(req api.Context) error {
	if !mcp.IsKubernetesBackend(h.mcpRuntimeBackend) {
		return req.Write(types.AppK8sSettings{})
	}

	settings, err := appK8sSettingsFromDeployment(
		req.Context(),
		h.localK8sClient,
		h.serviceNamespace,
		h.serviceName,
		appContainerName,
	)
	if err != nil {
		return err
	}

	return req.Write(settings)
}

func appK8sSettingsFromDeployment(ctx context.Context, k8sClient client.Client, namespace, deploymentName, containerName string) (types.AppK8sSettings, error) {
	if k8sClient == nil || namespace == "" || deploymentName == "" {
		return types.AppK8sSettings{}, nil
	}
	if containerName == "" {
		containerName = appContainerName
	}

	var deployment appsv1.Deployment
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: deploymentName}, &deployment); err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
			return types.AppK8sSettings{}, err
		}
		return types.AppK8sSettings{}, fmt.Errorf("failed to get deployment %s/%s: %w", namespace, deploymentName, err)
	}

	podSpec := deployment.Spec.Template.Spec

	var resources *corev1.ResourceRequirements
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == containerName {
			containerResources := podSpec.Containers[i].Resources
			if len(containerResources.Requests) > 0 || len(containerResources.Limits) > 0 {
				resources = &containerResources
			}
			break
		}
	}

	var runtimeClassName *string
	if podSpec.RuntimeClassName != nil && *podSpec.RuntimeClassName != "" {
		runtimeClassName = podSpec.RuntimeClassName
	}

	formatted, err := FormatPodSchedulingYAML(podSpec.Affinity, podSpec.Tolerations, resources, runtimeClassName)
	if err != nil {
		return types.AppK8sSettings{}, err
	}

	return types.AppK8sSettings{
		Affinity:         formatted.Affinity,
		Tolerations:      formatted.Tolerations,
		Resources:        formatted.Resources,
		RuntimeClassName: formatted.RuntimeClassName,
	}, nil
}

func (h *K8sSettingsHandler) Get(req api.Context) error {
	var settings v1.K8sSettings
	if err := req.Storage.Get(req.Context(), client.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.K8sSettingsName,
	}, &settings); err != nil {
		return err
	}

	converted, err := convertK8sSettings(settings)
	if err != nil {
		return err
	}

	return req.Write(converted)
}

func (h *K8sSettingsHandler) Defaults(req api.Context) error {
	var settings v1.K8sSettings
	if err := req.Storage.Get(req.Context(), client.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.K8sSettingsName,
	}, &settings); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	// Match the resources the Kubernetes backend will actually use when no
	// explicit defaults are configured. Resource maximums cap implicit fallbacks.
	return req.Write(convertResourceRequirements(mcp.EffectiveDefaultMCPResourceRequirementsWithMaximums(settings.Spec, h.mcpSessionManager.KubernetesResourceMaximums())))
}

func (h *K8sSettingsHandler) Update(req api.Context) error {
	var input types.K8sSettings
	if err := req.Read(&input); err != nil {
		return err
	}

	var (
		affinity              corev1.Affinity
		tolerations           []corev1.Toleration
		resources             corev1.ResourceRequirements
		nanobotAgentResources corev1.ResourceRequirements
		errs                  []error
	)

	if input.Affinity != "" {
		if err := yaml.UnmarshalStrict([]byte(input.Affinity), &affinity); err != nil {
			errs = append(errs, fmt.Errorf("invalid affinity YAML: %v", err))
		}
	}

	if input.Tolerations != "" {
		if err := yaml.UnmarshalStrict([]byte(input.Tolerations), &tolerations); err != nil {
			errs = append(errs, fmt.Errorf("invalid tolerations YAML: %v", err))
		}
	}

	if input.Resources != "" {
		if err := yaml.UnmarshalStrict([]byte(input.Resources), &resources); err != nil {
			errs = append(errs, fmt.Errorf("invalid resources YAML: %v", err))
		}
	}

	if input.NanobotAgentResources != "" {
		if err := yaml.UnmarshalStrict([]byte(input.NanobotAgentResources), &nanobotAgentResources); err != nil {
			errs = append(errs, fmt.Errorf("invalid nanobotAgentResources YAML: %v", err))
		}
	}

	if input.NanobotWorkspaceSize != "" {
		if _, err := resource.ParseQuantity(input.NanobotWorkspaceSize); err != nil {
			errs = append(errs, fmt.Errorf("invalid nanobotWorkspaceSize: %v", err))
		}
	}

	// Check for parsing errors before attempting any storage operations
	if len(errs) > 0 {
		return types.NewErrBadRequest("%v", errors.Join(errs...))
	}

	// Use retry.RetryOnConflict to handle ResourceVersion conflicts that can
	// occur when controllers (e.g. DetectK8sSettingsDrift) update the K8sSettings
	// object concurrently, or when two admins save settings at the same time.
	var settings v1.K8sSettings
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := req.Storage.Get(req.Context(), client.ObjectKey{
			Namespace: req.Namespace(),
			Name:      system.K8sSettingsName,
		}, &settings); err != nil {
			return err
		}

		// Don't allow updates if set via Helm
		if settings.Spec.SetViaHelm {
			return types.NewErrBadRequest("K8s settings are managed via Helm and cannot be updated through the API")
		}

		// PodSecurityAdmission settings are managed at initialization time (e.g. via Helm)
		// and are read-only via this API.
		//
		// To keep this behavior while allowing clients to submit broader update payloads
		// (for example, round-tripping settings they previously read), we ignore any
		// PodSecurityAdmission values provided in the request instead of rejecting the
		// entire update. The stored PodSecurityAdmission settings, if any, remain
		// unchanged and continue to be enforced by the system.
		// Note: input.PodSecurityAdmission is intentionally not processed here.

		// Update the settings object
		if input.Affinity != "" {
			settings.Spec.Affinity = &affinity
		} else {
			settings.Spec.Affinity = nil
		}

		if input.Tolerations != "" {
			settings.Spec.Tolerations = tolerations
		} else {
			settings.Spec.Tolerations = nil
		}

		if input.Resources != "" {
			settings.Spec.Resources = &resources
		} else {
			settings.Spec.Resources = nil
		}

		if input.RuntimeClassName != "" {
			settings.Spec.RuntimeClassName = &input.RuntimeClassName
		} else {
			settings.Spec.RuntimeClassName = nil
		}

		if input.StorageClassName != "" {
			settings.Spec.StorageClassName = &input.StorageClassName
		} else {
			settings.Spec.StorageClassName = nil
		}

		if input.NanobotWorkspaceSize != "" {
			settings.Spec.NanobotWorkspaceSize = input.NanobotWorkspaceSize
		} else {
			settings.Spec.NanobotWorkspaceSize = ""
		}

		if input.NanobotAgentResources != "" {
			settings.Spec.NanobotAgentResources = &nanobotAgentResources
		} else {
			settings.Spec.NanobotAgentResources = nil
		}

		if err := validateK8sSettingsResourceMaximums(h.mcpSessionManager, settings.Spec); err != nil {
			return err
		}

		return req.Storage.Update(req.Context(), &settings)
	}); err != nil {
		return err
	}

	converted, err := convertK8sSettings(settings)
	if err != nil {
		return err
	}

	return req.Write(converted)
}

func convertResourceRequirements(resources corev1.ResourceRequirements) *types.MCPResourceRequirements {
	result := &types.MCPResourceRequirements{}
	if cpu, ok := resources.Requests[corev1.ResourceCPU]; ok {
		result.Requests.CPU = cpu.String()
	}
	if memory, ok := resources.Requests[corev1.ResourceMemory]; ok {
		result.Requests.Memory = memory.String()
	}
	if cpu, ok := resources.Limits[corev1.ResourceCPU]; ok {
		result.Limits.CPU = cpu.String()
	}
	if memory, ok := resources.Limits[corev1.ResourceMemory]; ok {
		result.Limits.Memory = memory.String()
	}
	return result
}

func convertK8sSettings(settings v1.K8sSettings) (types.K8sSettings, error) {
	result := types.K8sSettings{
		SetViaHelm: settings.Spec.SetViaHelm,
		Metadata:   MetadataFrom(&settings),
	}

	formatted, err := FormatPodSchedulingYAML(
		settings.Spec.Affinity,
		settings.Spec.Tolerations,
		settings.Spec.Resources,
		settings.Spec.RuntimeClassName,
	)
	if err != nil {
		return types.K8sSettings{}, err
	}
	result.Affinity = formatted.Affinity
	result.Tolerations = formatted.Tolerations
	result.Resources = formatted.Resources
	result.RuntimeClassName = formatted.RuntimeClassName

	if settings.Spec.StorageClassName != nil {
		result.StorageClassName = *settings.Spec.StorageClassName
	}

	if settings.Spec.NanobotWorkspaceSize != "" {
		result.NanobotWorkspaceSize = settings.Spec.NanobotWorkspaceSize
	}

	if settings.Spec.NanobotAgentResources != nil {
		nanobotAgentResourcesYAML, err := yaml.Marshal(settings.Spec.NanobotAgentResources)
		if err != nil {
			return types.K8sSettings{}, err
		}
		result.NanobotAgentResources = string(nanobotAgentResourcesYAML)
	}

	// Convert PSA settings
	if settings.Spec.PodSecurityAdmission != nil {
		result.PodSecurityAdmission = &types.PodSecurityAdmissionSettings{
			Enabled:        settings.Spec.PodSecurityAdmission.Enabled,
			Enforce:        settings.Spec.PodSecurityAdmission.Enforce,
			EnforceVersion: settings.Spec.PodSecurityAdmission.EnforceVersion,
			Audit:          settings.Spec.PodSecurityAdmission.Audit,
			AuditVersion:   settings.Spec.PodSecurityAdmission.AuditVersion,
			Warn:           settings.Spec.PodSecurityAdmission.Warn,
			WarnVersion:    settings.Spec.PodSecurityAdmission.WarnVersion,
		}
	}

	return result, nil
}

// PodSchedulingYAML contains the shared pod scheduling fields returned by the API.
type PodSchedulingYAML struct {
	Affinity         string
	Tolerations      string
	Resources        string
	RuntimeClassName string
}

// FormatPodSchedulingYAML converts parsed pod scheduling fields into API YAML strings.
func FormatPodSchedulingYAML(
	affinity *corev1.Affinity,
	tolerations []corev1.Toleration,
	resources *corev1.ResourceRequirements,
	runtimeClassName *string,
) (PodSchedulingYAML, error) {
	var result PodSchedulingYAML

	if affinity != nil {
		affinityYAML, err := yaml.Marshal(affinity)
		if err != nil {
			return PodSchedulingYAML{}, err
		}
		result.Affinity = string(affinityYAML)
	}

	if len(tolerations) > 0 {
		tolerationsYAML, err := yaml.Marshal(tolerations)
		if err != nil {
			return PodSchedulingYAML{}, err
		}
		result.Tolerations = string(tolerationsYAML)
	}

	if resources != nil {
		resourcesYAML, err := yaml.Marshal(resources)
		if err != nil {
			return PodSchedulingYAML{}, err
		}
		result.Resources = string(resourcesYAML)
	}

	if runtimeClassName != nil {
		result.RuntimeClassName = *runtimeClassName
	}

	return result, nil
}
