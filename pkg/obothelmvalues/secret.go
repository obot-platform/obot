package obothelmvalues

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const secretKeyValuesYAML = "values.yaml"

// SecretName returns the Kubernetes secret name for the Helm values snapshot.
func SecretName(serviceName string) string {
	return strings.TrimSpace(serviceName) + "-helm-values"
}

// MaskedValuesFromSecret reads the Helm values snapshot secret and returns masked values
// suitable for display. Returns nil when the secret is missing, forbidden, or empty.
func MaskedValuesFromSecret(ctx context.Context, localK8sClient kclient.Client, serviceNamespace, serviceName string) (map[string]any, error) {
	if localK8sClient == nil {
		return nil, nil
	}

	namespace := strings.TrimSpace(serviceNamespace)
	name := strings.TrimSpace(serviceName)
	if namespace == "" || name == "" {
		return nil, nil
	}

	var secret corev1.Secret
	if err := localK8sClient.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: SecretName(name)}, &secret); err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get helm values secret %s/%s: %w", namespace, SecretName(name), err)
	}

	valuesYAML := strings.TrimSpace(string(secret.Data[secretKeyValuesYAML]))
	if valuesYAML == "" {
		return nil, nil
	}

	maskedYAML, err := MaskValuesYAML(valuesYAML)
	if err != nil {
		return nil, err
	}
	if maskedYAML == "" {
		return nil, nil
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(maskedYAML), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse masked helm values: %w", err)
	}
	return parsed, nil
}
