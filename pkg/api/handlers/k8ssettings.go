package handlers

import (
	"errors"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
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

	var settings v1.K8sSettings
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

	// Check for earlier parsing errors
	if len(errs) > 0 {
		return types.NewErrBadRequest("%v", errors.Join(errs...))
	}

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

	if err := req.Storage.Update(req.Context(), &settings); err != nil {
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

	return result, nil
}
