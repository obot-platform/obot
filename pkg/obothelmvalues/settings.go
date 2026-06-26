package obothelmvalues

import (
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"sigs.k8s.io/yaml"
)

// ParseObotK8sSettings builds the API response from masked Helm values.
// Values must already be masked (via MaskedValuesFromSecret); this function does not redact secrets.
func ParseObotK8sSettings(values map[string]any) (types.ObotK8sSettings, error) {
	result := types.ObotK8sSettings{Available: true}

	if replicaCount, ok := scalarInt32(values["replicaCount"]); ok {
		result.ReplicaCount = &replicaCount
	}

	result.UpdateStrategy = scalarString(values["updateStrategy"])
	result.RuntimeClassName = scalarString(values["runtimeClassName"])

	for _, key := range yamlSectionKeys {
		sectionYAML, err := sectionYAML(values[key])
		if err != nil {
			return types.ObotK8sSettings{}, fmt.Errorf("failed to marshal %q: %w", key, err)
		}
		if sectionYAML == "" {
			continue
		}
		if err := setYAMLSection(&result, key, sectionYAML); err != nil {
			return types.ObotK8sSettings{}, err
		}
	}

	return result, nil
}

func setYAMLSection(result *types.ObotK8sSettings, key, value string) error {
	switch key {
	case "dev":
		result.Dev = value
	case "image":
		result.Image = value
	case "imagePullSecrets":
		result.ImagePullSecrets = value
	case "additionalLabels":
		result.AdditionalLabels = value
	case "podAnnotations":
		result.PodAnnotations = value
	case "service":
		result.Service = value
	case "ingress":
		result.Ingress = value
	case "config":
		result.Config = value
	case "resources":
		result.Resources = value
	case "persistence":
		result.Persistence = value
	case "extraVolumes":
		result.ExtraVolumes = value
	case "extraVolumeMounts":
		result.ExtraVolumeMounts = value
	case "serviceAccount":
		result.ServiceAccount = value
	case "nodeSelector":
		result.NodeSelector = value
	case "tolerations":
		result.Tolerations = value
	case "affinity":
		result.Affinity = value
	default:
		return fmt.Errorf("unknown YAML section key %q", key)
	}
	return nil
}

func sectionYAML(value any) (string, error) {
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
