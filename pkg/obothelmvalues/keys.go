package obothelmvalues

// ConfigurableTopLevelKeys lists Helm .Values keys captured at install/upgrade time
// in chart/templates/_helpers.tpl (obot.helmValuesSnapshot).
//
// When adding or removing configurable fields, update both this slice and that template.
// mcpImagePullSecrets and mcpServerDefaults are intentionally excluded.
var ConfigurableTopLevelKeys = []string{
	"replicaCount",
	"dev",
	"image",
	"imagePullSecrets",
	"updateStrategy",
	"additionalLabels",
	"podAnnotations",
	"service",
	"ingress",
	"config",
	"resources",
	"runtimeClassName",
	"persistence",
	"extraVolumes",
	"extraVolumeMounts",
	"serviceAccount",
	"nodeSelector",
	"tolerations",
	"affinity",
}

// yamlSectionKeys are configurable keys surfaced as YAML strings on ObotK8sSettings.
var yamlSectionKeys = []string{
	"dev",
	"image",
	"imagePullSecrets",
	"additionalLabels",
	"podAnnotations",
	"service",
	"ingress",
	"config",
	"resources",
	"persistence",
	"extraVolumes",
	"extraVolumeMounts",
	"serviceAccount",
	"nodeSelector",
	"tolerations",
	"affinity",
}

// sectionsWithAnnotationMaps are top-level keys whose nested "annotations" map is masked.
var sectionsWithAnnotationMaps = map[string]struct{}{
	"service":        {},
	"ingress":        {},
	"serviceAccount": {},
}

func pickConfigurableValues(values map[string]any) map[string]any {
	if len(values) == 0 {
		return values
	}

	picked := make(map[string]any, len(ConfigurableTopLevelKeys))
	for _, key := range ConfigurableTopLevelKeys {
		value, ok := values[key]
		if !ok {
			continue
		}
		picked[key] = value
	}
	return picked
}
