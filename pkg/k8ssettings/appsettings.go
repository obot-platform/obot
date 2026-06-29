package k8ssettings

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
)

// ParseAppK8sSettings builds the app scheduling API response from Helm values.
func ParseAppK8sSettings(values map[string]any) (types.AppK8sSettings, error) {
	result := types.AppK8sSettings{Available: true}

	runtimeClassName := scalarString(values["runtimeClassName"])
	if runtimeClassName != "" {
		result.RuntimeClassName = runtimeClassName
	}

	for _, key := range []string{"affinity", "tolerations", "resources"} {
		yamlValue, err := sectionYAML(values[key])
		if err != nil {
			return types.AppK8sSettings{}, fmt.Errorf("failed to marshal %q: %w", key, err)
		}
		if yamlValue == "" {
			continue
		}
		switch key {
		case "affinity":
			result.Affinity = yamlValue
		case "tolerations":
			result.Tolerations = yamlValue
		case "resources":
			result.Resources = yamlValue
		}
	}

	return result, nil
}
