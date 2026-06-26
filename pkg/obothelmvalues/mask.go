package obothelmvalues

import (
	"fmt"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"
)

const MaskedValue = "****"

// MaskValuesYAML parses a Helm values snapshot and returns a display-safe YAML string.
// This is the only masking entry point for Helm values displayed via GET /api/k8s-settings.
func MaskValuesYAML(valuesYAML string) (string, error) {
	valuesYAML = strings.TrimSpace(valuesYAML)
	if valuesYAML == "" {
		return "", nil
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(valuesYAML), &parsed); err != nil {
		return "", fmt.Errorf("failed to parse helm values for masking: %w", err)
	}

	masked, err := yaml.Marshal(MaskValues(parsed))
	if err != nil {
		return "", fmt.Errorf("failed to marshal masked helm values: %w", err)
	}
	return strings.TrimSpace(string(masked)), nil
}

// MaskValues returns a display-safe copy of configurable Helm values.
func MaskValues(values map[string]any) map[string]any {
	values = pickConfigurableValues(values)
	if len(values) == 0 {
		return values
	}

	masked := make(map[string]any, len(values))
	for key, value := range values {
		switch key {
		case "config":
			masked[key] = maskConfigSection(value)
		case "podAnnotations":
			masked[key] = maskStringMap(value)
		default:
			if _, ok := sectionsWithAnnotationMaps[key]; ok {
				masked[key] = maskSectionWithAnnotations(value)
				continue
			}
			masked[key] = value
		}
	}
	return masked
}

func maskConfigSection(value any) any {
	configMap, ok := toStringAnyMap(value)
	if !ok {
		return value
	}

	masked := make(map[string]any)
	for key, val := range configMap {
		if isEmptyConfigValue(val) {
			continue
		}
		masked[key] = MaskedValue
	}
	return masked
}

func isEmptyConfigValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return typed == ""
	default:
		return false
	}
}

func maskStringMap(value any) any {
	stringMap, ok := toStringMap(value)
	if !ok {
		return value
	}
	return maskStringMapValues(stringMap)
}

func maskSectionWithAnnotations(value any) any {
	section, ok := toStringAnyMap(value)
	if !ok {
		return value
	}
	result := copyStringAnyMap(section)
	if annotations, ok := result["annotations"]; ok {
		result["annotations"] = maskStringMap(annotations)
	}
	return result
}

func copyStringAnyMap(values map[string]any) map[string]any {
	copied := make(map[string]any, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func toStringAnyMap(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		return typed, true
	case map[any]any:
		converted := make(map[string]any, len(typed))
		for key, val := range typed {
			converted[fmt.Sprint(key)] = val
		}
		return converted, true
	default:
		return nil, false
	}
}

func toStringMap(value any) (map[string]string, bool) {
	switch typed := value.(type) {
	case map[string]string:
		return typed, true
	case map[string]any:
		converted := make(map[string]string, len(typed))
		for key, val := range typed {
			converted[key] = scalarString(val)
		}
		return converted, true
	case map[any]any:
		converted := make(map[string]string, len(typed))
		for key, val := range typed {
			converted[fmt.Sprint(key)] = scalarString(val)
		}
		return converted, true
	default:
		return nil, false
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

func maskStringMapValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	masked := make(map[string]string, len(values))
	for key, value := range values {
		if isSensitiveSettingKey(key) || value != "" {
			masked[key] = MaskedValue
			continue
		}
		masked[key] = value
	}
	return masked
}

func isSensitiveSettingKey(key string) bool {
	lowerKey := strings.ToLower(key)
	sensitiveFragments := []string{
		"password",
		"secret",
		"token",
		"apikey",
		"api_key",
		"api-key",
		"email",
		"phone",
		"credential",
		"auth",
		"license",
		"key",
		"dsn",
	}
	for _, fragment := range sensitiveFragments {
		if strings.Contains(lowerKey, fragment) {
			return true
		}
	}
	return false
}
