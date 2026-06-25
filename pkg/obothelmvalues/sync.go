package obothelmvalues

import (
	"context"
	"fmt"
	"strings"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const secretKeyValuesYAML = "values.yaml"

// SecretName returns the Kubernetes secret name for the Helm values snapshot.
func SecretName(serviceName string) string {
	return strings.TrimSpace(serviceName) + "-helm-values"
}

// SyncFromSecret reads the Helm values snapshot secret and stores it in ObotHelmValues.
func SyncFromSecret(ctx context.Context, storageClient, localK8sClient kclient.Client, serviceNamespace, serviceName string) error {
	if localK8sClient == nil {
		return nil
	}

	namespace := strings.TrimSpace(serviceNamespace)
	name := strings.TrimSpace(serviceName)
	if namespace == "" || name == "" {
		return nil
	}

	var secret corev1.Secret
	if err := localK8sClient.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: SecretName(name)}, &secret); err != nil {
		if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
			return nil
		}
		return fmt.Errorf("failed to get helm values secret %s/%s: %w", namespace, SecretName(name), err)
	}

	valuesYAML := strings.TrimSpace(string(secret.Data[secretKeyValuesYAML]))
	if valuesYAML == "" {
		return nil
	}

	maskedYAML, err := MaskValuesYAML(valuesYAML)
	if err != nil {
		return err
	}

	var existing v1.ObotHelmValues
	if err := storageClient.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.ObotHelmValuesName,
	}, &existing); apierrors.IsNotFound(err) {
		return storageClient.Create(ctx, &v1.ObotHelmValues{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.ObotHelmValuesName,
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.ObotHelmValuesSpec{
				ValuesYAML: maskedYAML,
			},
		})
	} else if err != nil {
		return fmt.Errorf("failed to get obot helm values: %w", err)
	}

	if existing.Spec.ValuesYAML == maskedYAML {
		return nil
	}

	existing.Spec.ValuesYAML = maskedYAML
	return storageClient.Update(ctx, &existing)
}
