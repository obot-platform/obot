package substrate

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func ValidateWorkerScheduling(selector map[string]string, tolerations []corev1.Toleration) error {
	for key, value := range selector {
		if len(validation.IsQualifiedName(key)) != 0 || len(validation.IsValidLabelValue(value)) != 0 {
			return fmt.Errorf("substrate worker node selector must contain valid Kubernetes labels")
		}
	}

	if len(tolerations) > 16 {
		return fmt.Errorf("at most 16 Substrate worker tolerations are allowed")
	}
	for _, toleration := range tolerations {
		if toleration.Key != "" && len(validation.IsQualifiedName(toleration.Key)) != 0 {
			return fmt.Errorf("invalid Substrate worker toleration key %q", toleration.Key)
		}
		if toleration.Key == "" && toleration.Operator != corev1.TolerationOpExists {
			return fmt.Errorf("substrate worker tolerations with an empty key must use Exists")
		}

		switch toleration.Operator {
		case "", corev1.TolerationOpEqual:
			if len(validation.IsValidLabelValue(toleration.Value)) != 0 {
				return fmt.Errorf("invalid Substrate worker toleration value %q", toleration.Value)
			}
		case corev1.TolerationOpExists:
			if toleration.Value != "" {
				return fmt.Errorf("substrate worker Exists tolerations must not specify a value")
			}
		default:
			return fmt.Errorf("substrate worker toleration operator must be Equal or Exists")
		}

		switch toleration.Effect {
		case "", corev1.TaintEffectNoSchedule, corev1.TaintEffectPreferNoSchedule, corev1.TaintEffectNoExecute:
		default:
			return fmt.Errorf("invalid Substrate worker toleration effect %q", toleration.Effect)
		}
		if toleration.TolerationSeconds != nil && toleration.Effect != corev1.TaintEffectNoExecute {
			return fmt.Errorf("substrate worker tolerationSeconds requires the NoExecute effect")
		}
	}

	return nil
}
