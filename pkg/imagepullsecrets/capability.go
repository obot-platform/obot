package imagepullsecrets

import "strings"

const (
	CredentialContext = "image-pull-secrets"
	PasswordEnvVar    = "REGISTRY_PASSWORD"

	DefaultECRAudience        = "sts.amazonaws.com"
	DefaultECRRefreshSchedule = "0 */6 * * *"

	LabelManagedBy       = "obot.ai/managed-by"
	LabelManagedByValue  = "image-pull-secrets"
	LabelImagePullSecret = "obot.ai/image-pull-secret"

	AnnotationECRRefreshRequestedAt = "obot.ai/ecr-refresh-requested-at"
)

type Capability struct {
	Available bool
	Reason    string
}

func Availability(kubernetesBackend bool, staticPullSecrets []string) Capability {
	if !kubernetesBackend {
		return Capability{
			Reason: "managed image pull secrets require the Kubernetes MCP runtime backend",
		}
	}

	if len(CleanSecretNames(staticPullSecrets)) > 0 {
		return Capability{
			Reason: "static MCP image pull secrets are configured",
		}
	}

	return Capability{Available: true}
}

func ECRSubject(namespace, serviceAccountName string) string {
	namespace = strings.TrimSpace(namespace)
	serviceAccountName = strings.TrimSpace(serviceAccountName)
	if namespace == "" || serviceAccountName == "" {
		return ""
	}
	return "system:serviceaccount:" + namespace + ":" + serviceAccountName
}
