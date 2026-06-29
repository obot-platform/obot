package k8ssettings

import (
	"fmt"
	"strings"

	"sigs.k8s.io/yaml"
)

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

func scalarString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}
