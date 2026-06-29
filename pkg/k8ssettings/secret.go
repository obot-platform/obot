package k8ssettings

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const secretKeyAppK8sSettingsYAML = "app-k8s-settings.yaml"

// AppK8sSettingsValuesFromSecret reads the app scheduling snapshot from the config secret.
// Returns nil when the secret is missing, forbidden, or empty.
func AppK8sSettingsValuesFromSecret(ctx context.Context, localK8sClient kclient.Client, namespace, secretName string) (map[string]any, error) {
	if localK8sClient == nil {
		return nil, nil
	}

	namespace = strings.TrimSpace(namespace)
	secretName = strings.TrimSpace(secretName)
	if namespace == "" || secretName == "" {
		return nil, nil
	}

	var secret corev1.Secret
	if err := localK8sClient.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: secretName}, &secret); err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get config secret %s/%s: %w", namespace, secretName, err)
	}

	valuesYAML := strings.TrimSpace(string(secret.Data[secretKeyAppK8sSettingsYAML]))
	if valuesYAML == "" {
		return nil, nil
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(valuesYAML), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse app k8s settings from config secret: %w", err)
	}
	return parsed, nil
}
