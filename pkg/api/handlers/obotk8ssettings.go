package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/obothelmvalues"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

func (h *K8sSettingsHandler) buildObotK8sSettings(ctx context.Context, storage kclient.Client) (types.ObotK8sSettings, error) {
	if !mcp.IsKubernetesBackend(h.mcpRuntimeBackend) {
		return types.ObotK8sSettings{Available: false}, nil
	}

	if err := obothelmvalues.SyncFromSecret(ctx, storage, h.localK8sClient, h.serviceNamespace, h.serviceName); err != nil {
		return types.ObotK8sSettings{}, err
	}

	var helmValues v1.ObotHelmValues
	if err := storage.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.ObotHelmValuesName,
	}, &helmValues); err != nil {
		if apierrors.IsNotFound(err) {
			return types.ObotK8sSettings{Available: false}, nil
		}
		return types.ObotK8sSettings{}, fmt.Errorf("failed to get obot helm values: %w", err)
	}

	valuesYAML := strings.TrimSpace(helmValues.Spec.ValuesYAML)
	if valuesYAML == "" {
		return types.ObotK8sSettings{Available: false}, nil
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(valuesYAML), &parsed); err != nil {
		return types.ObotK8sSettings{}, fmt.Errorf("failed to parse stored helm values: %w", err)
	}

	return buildObotK8sSettingsFromHelmValues(obothelmvalues.MaskValues(parsed))
}

func buildObotK8sSettingsFromHelmValues(values map[string]any) (types.ObotK8sSettings, error) {
	result := types.ObotK8sSettings{Available: true}

	if replicaCount, ok := scalarInt32(values["replicaCount"]); ok {
		result.ReplicaCount = &replicaCount
	}

	result.UpdateStrategy = scalarString(values["updateStrategy"])
	result.RuntimeClassName = scalarString(values["runtimeClassName"])

	if devYAML, err := helmSectionYAML(values["dev"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if devYAML != "" {
		result.Dev = devYAML
	}

	if imageYAML, err := helmSectionYAML(values["image"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if imageYAML != "" {
		result.Image = imageYAML
	}

	if imagePullSecretsYAML, err := helmSectionYAML(values["imagePullSecrets"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if imagePullSecretsYAML != "" {
		result.ImagePullSecrets = imagePullSecretsYAML
	}

	if additionalLabelsYAML, err := helmSectionYAML(values["additionalLabels"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if additionalLabelsYAML != "" {
		result.AdditionalLabels = additionalLabelsYAML
	}

	if podAnnotationsYAML, err := helmSectionYAML(values["podAnnotations"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if podAnnotationsYAML != "" {
		result.PodAnnotations = podAnnotationsYAML
	}

	if serviceYAML, err := helmSectionYAML(values["service"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if serviceYAML != "" {
		result.Service = serviceYAML
	}

	if ingressYAML, err := helmSectionYAML(values["ingress"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if ingressYAML != "" {
		result.Ingress = ingressYAML
	}

	if configYAML, err := helmSectionYAML(values["config"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if configYAML != "" {
		result.Config = configYAML
	}

	if resourcesYAML, err := helmSectionYAML(values["resources"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if resourcesYAML != "" {
		result.Resources = resourcesYAML
	}

	if persistenceYAML, err := helmSectionYAML(values["persistence"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if persistenceYAML != "" {
		result.Persistence = persistenceYAML
	}

	if extraVolumesYAML, err := helmSectionYAML(values["extraVolumes"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if extraVolumesYAML != "" {
		result.ExtraVolumes = extraVolumesYAML
	}

	if extraVolumeMountsYAML, err := helmSectionYAML(values["extraVolumeMounts"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if extraVolumeMountsYAML != "" {
		result.ExtraVolumeMounts = extraVolumeMountsYAML
	}

	if serviceAccountYAML, err := helmSectionYAML(values["serviceAccount"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if serviceAccountYAML != "" {
		result.ServiceAccount = serviceAccountYAML
	}

	if nodeSelectorYAML, err := helmSectionYAML(values["nodeSelector"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if nodeSelectorYAML != "" {
		result.NodeSelector = nodeSelectorYAML
	}

	if tolerationsYAML, err := helmSectionYAML(values["tolerations"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if tolerationsYAML != "" {
		result.Tolerations = tolerationsYAML
	}

	if affinityYAML, err := helmSectionYAML(values["affinity"]); err != nil {
		return types.ObotK8sSettings{}, err
	} else if affinityYAML != "" {
		result.Affinity = affinityYAML
	}

	return result, nil
}

func helmSectionYAML(value any) (string, error) {
	if value == nil || isEmptyHelmValue(value) {
		return "", nil
	}

	data, err := yaml.Marshal(value)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func isEmptyHelmValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	case map[string]any:
		return len(typed) == 0
	case map[any]any:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	default:
		return false
	}
}

func scalarString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case float64:
		return strconv.FormatInt(int64(typed), 10)
	default:
		return fmt.Sprint(value)
	}
}

func scalarInt32(value any) (int32, bool) {
	switch typed := value.(type) {
	case int:
		return int32(typed), true
	case int64:
		return int32(typed), true
	case float64:
		return int32(typed), true
	default:
		return 0, false
	}
}
