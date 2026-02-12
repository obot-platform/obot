package handlers

import (
	"errors"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type K8sSettingsHandler struct{}

func NewK8sSettingsHandler() *K8sSettingsHandler {
	return &K8sSettingsHandler{}
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

func (h *K8sSettingsHandler) Update(req api.Context) error {
	var input types.K8sSettings
	if err := req.Read(&input); err != nil {
		return err
	}

	var (
		affinity    corev1.Affinity
		tolerations []corev1.Toleration
		resources   corev1.ResourceRequirements
		errs        []error
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

func convertK8sSettings(settings v1.K8sSettings) (types.K8sSettings, error) {
	result := types.K8sSettings{
		SetViaHelm: settings.Spec.SetViaHelm,
		Metadata:   MetadataFrom(&settings),
	}

	if settings.Spec.Affinity != nil {
		affinityYAML, err := yaml.Marshal(settings.Spec.Affinity)
		if err != nil {
			return types.K8sSettings{}, err
		}
		result.Affinity = string(affinityYAML)
	}

	if len(settings.Spec.Tolerations) > 0 {
		tolerationsYAML, err := yaml.Marshal(settings.Spec.Tolerations)
		if err != nil {
			return types.K8sSettings{}, err
		}
		result.Tolerations = string(tolerationsYAML)
	}

	if settings.Spec.Resources != nil {
		resourcesYAML, err := yaml.Marshal(settings.Spec.Resources)
		if err != nil {
			return types.K8sSettings{}, err
		}
		result.Resources = string(resourcesYAML)
	}

	if settings.Spec.RuntimeClassName != nil {
		result.RuntimeClassName = *settings.Spec.RuntimeClassName
	}

	if settings.Spec.StorageClassName != nil {
		result.StorageClassName = *settings.Spec.StorageClassName
	}

	if settings.Spec.NanobotWorkspaceSize != "" {
		result.NanobotWorkspaceSize = settings.Spec.NanobotWorkspaceSize
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
