package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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

	// Retry loop to handle conflicts
	var settings v1.K8sSettings
	for i := 0; i < 5; i++ {
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

		// Parse and update affinity
		if input.Affinity != "" {
			var affinity corev1.Affinity
			if err := yaml.Unmarshal([]byte(input.Affinity), &affinity); err != nil {
				return types.NewErrBadRequest("invalid affinity YAML: %v", err)
			}
			settings.Spec.Affinity = &affinity
		} else {
			settings.Spec.Affinity = nil
		}

		// Parse and update tolerations
		if input.Tolerations != "" {
			var tolerations []corev1.Toleration
			if err := yaml.Unmarshal([]byte(input.Tolerations), &tolerations); err != nil {
				return types.NewErrBadRequest("invalid tolerations YAML: %v", err)
			}
			settings.Spec.Tolerations = tolerations
		} else {
			settings.Spec.Tolerations = nil
		}

		// Parse and update resources
		if input.Resources != "" {
			var resources corev1.ResourceRequirements
			if err := yaml.Unmarshal([]byte(input.Resources), &resources); err != nil {
				return types.NewErrBadRequest("invalid resources YAML: %v", err)
			}
			settings.Spec.Resources = &resources
		} else {
			settings.Spec.Resources = nil
		}

		// Try to update - if conflict, retry
		err := req.Storage.Update(req.Context(), &settings)
		if err == nil {
			// Success!
			break
		}
		if !apierrors.IsConflict(err) {
			// Non-conflict error, return immediately
			return err
		}
		// Conflict error - loop will retry
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

	return result, nil
}
