package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/imagepullsecrets"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var defaultECRPolicyJSON = buildECRPolicyJSON()

type ImagePullSecretHandler struct {
	mcpRuntimeBackend  string
	staticSecrets      []string
	mcpNamespace       string
	serviceNamespace   string
	serviceAccountName string
	runtimeClient      kclient.Client
	issuerURL          string
	issuerError        string
	ecrPolicyJSON      string
}

func NewImagePullSecretHandler(mcpRuntimeBackend string, staticSecrets []string, mcpNamespace, serviceNamespace, serviceAccountName string, runtimeClient kclient.Client, issuerURL, issuerError string) *ImagePullSecretHandler {
	return &ImagePullSecretHandler{
		mcpRuntimeBackend:  mcpRuntimeBackend,
		staticSecrets:      staticSecrets,
		mcpNamespace:       mcpNamespace,
		serviceNamespace:   firstNonEmpty(serviceNamespace, mcpNamespace),
		serviceAccountName: strings.TrimSpace(serviceAccountName),
		runtimeClient:      runtimeClient,
		issuerURL:          issuerURL,
		issuerError:        issuerError,
		ecrPolicyJSON:      defaultECRPolicyJSON,
	}
}

func (h *ImagePullSecretHandler) Capability(req api.Context) error {
	return req.Write(h.convertCapability())
}

func (h *ImagePullSecretHandler) List(req api.Context) error {
	var list v1.ImagePullSecretList
	if err := req.List(&list); err != nil {
		return fmt.Errorf("failed to list image pull secrets: %w", err)
	}

	passwordConfigured, err := listPasswordConfigured(req, list.Items)
	if err != nil {
		return fmt.Errorf("failed to check password configured: %w", err)
	}

	items := make([]types.ImagePullSecret, 0, len(list.Items))
	for _, item := range list.Items {
		converted := h.convert(item)
		if item.Spec.Basic != nil {
			converted.Status.PasswordConfigured = passwordConfigured[item.Name]
		}
		items = append(items, converted)
	}

	return req.Write(types.ImagePullSecretList{Items: items})
}

func (h *ImagePullSecretHandler) Get(req api.Context) error {
	var secret v1.ImagePullSecret
	if err := req.Get(&secret, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get image pull secret: %w", err)
	}

	converted := h.convert(secret)
	if secret.Spec.Basic != nil {
		if err := setPasswordConfigured(req, secret.Name, &converted.Status); err != nil {
			return fmt.Errorf("failed to check password configured: %w", err)
		}
	}
	return req.Write(converted)
}

func (h *ImagePullSecretHandler) Create(req api.Context) error {
	if err := h.ensureAvailable(); err != nil {
		return err
	}

	var input types.ImagePullSecretManifest
	if err := req.Read(&input); err != nil {
		return types.NewErrBadRequest("failed to read image pull secret manifest: %v", err)
	}

	spec, err := h.specFromInput(input, nil)
	if err != nil {
		return err
	}
	password := basicImagePullSecretPassword(input)
	if spec.Type == types.ImagePullSecretTypeBasic && password == "" {
		return types.NewErrBadRequest("password is required")
	}

	secret := v1.ImagePullSecret{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ImagePullSecretPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: spec,
	}

	if err := req.Create(&secret); err != nil {
		return fmt.Errorf("failed to create image pull secret: %w", err)
	}

	if spec.Type == types.ImagePullSecretTypeBasic {
		if err := storeImagePullSecretPassword(req, secret.Name, password); err != nil {
			_ = req.Delete(&secret)
			return fmt.Errorf("failed to store image pull secret password: %w", err)
		}
	}

	converted := h.convert(secret)
	if spec.Type == types.ImagePullSecretTypeBasic {
		converted.Status.PasswordConfigured = true
	}
	return req.WriteCreated(converted)
}

func (h *ImagePullSecretHandler) Update(req api.Context) error {
	if err := h.ensureAvailable(); err != nil {
		return err
	}

	var input types.ImagePullSecretManifest
	if err := req.Read(&input); err != nil {
		return types.NewErrBadRequest("failed to read image pull secret manifest: %v", err)
	}

	var existing v1.ImagePullSecret
	if err := req.Get(&existing, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get image pull secret: %w", err)
	}

	if input.Type != "" && input.Type != existing.Spec.Type {
		return types.NewErrBadRequest("type is immutable")
	}

	spec, err := h.specFromInput(input, &existing)
	if err != nil {
		return err
	}
	password := basicImagePullSecretPassword(input)
	if spec.Type == types.ImagePullSecretTypeBasic && password == "" {
		configured, err := passwordConfigured(req, existing.Name)
		if err != nil {
			return err
		}
		if !configured {
			return types.NewErrBadRequest("password is required")
		}
	}

	existing.Spec = spec
	if err := req.Update(&existing); err != nil {
		return fmt.Errorf("failed to update image pull secret: %w", err)
	}

	if spec.Type == types.ImagePullSecretTypeBasic && password != "" {
		if err := storeImagePullSecretPassword(req, existing.Name, password); err != nil {
			return fmt.Errorf("failed to store image pull secret password: %w", err)
		}
	}

	converted := h.convert(existing)
	if existing.Spec.Basic != nil {
		if err := setPasswordConfigured(req, existing.Name, &converted.Status); err != nil {
			return fmt.Errorf("failed to check password configured: %w", err)
		}
	}

	return req.Write(converted)
}

func (h *ImagePullSecretHandler) Delete(req api.Context) error {
	if err := h.ensureAvailable(); err != nil {
		return err
	}

	var secret v1.ImagePullSecret
	if err := req.Get(&secret, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get image pull secret: %w", err)
	}

	if err := deleteImagePullSecretPassword(req, secret.Name); err != nil {
		return fmt.Errorf("failed to delete image pull secret password: %w", err)
	}
	if err := req.Delete(&secret); err != nil {
		return fmt.Errorf("failed to delete image pull secret: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *ImagePullSecretHandler) Test(req api.Context) error {
	if err := h.ensureAvailable(); err != nil {
		return err
	}

	var input types.ImagePullSecretTestRequest
	if req.ContentLength != 0 {
		if err := req.Read(&input); err != nil {
			return types.NewErrBadRequest("failed to read image pull secret test request: %v", err)
		}
	}

	var secret v1.ImagePullSecret
	if err := req.Get(&secret, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get image pull secret: %w", err)
	}

	switch secret.Spec.Type {
	case types.ImagePullSecretTypeBasic:
		if secret.Spec.Basic == nil {
			return types.NewErrBadRequest("basic image pull secret configuration is missing")
		}
		if strings.TrimSpace(input.Image) == "" {
			return types.NewErrBadRequest("image is required")
		}
		password, err := revealImagePullSecretPassword(req, secret.Name)
		if err != nil {
			return err
		}
		result, err := imagepullsecrets.TestBasicRegistryCredentials(req.Context(), secret.Spec.Basic.Server, secret.Spec.Basic.Username, password, input.Image)
		if err != nil {
			return types.NewErrBadRequest("image pull secret test failed: %v", err)
		}
		return req.Write(types.ImagePullSecretTestResponse{Success: result.Success, Message: result.Message})
	case types.ImagePullSecretTypeECR:
		if strings.TrimSpace(input.Image) == "" {
			return types.NewErrBadRequest("image is required")
		}
		result, err := h.testECRImagePullSecret(req, secret, input.Image)
		if err != nil {
			return err
		}
		return req.Write(types.ImagePullSecretTestResponse{Success: result.Success, Message: result.Message})
	default:
		return types.NewErrBadRequest("unsupported image pull secret type %q", secret.Spec.Type)
	}
}

func (h *ImagePullSecretHandler) Refresh(req api.Context) error {
	if err := h.ensureAvailable(); err != nil {
		return err
	}

	var secret v1.ImagePullSecret
	if err := req.Get(&secret, req.PathValue("id")); err != nil {
		return fmt.Errorf("failed to get image pull secret: %w", err)
	}
	if secret.Spec.Type != types.ImagePullSecretTypeECR {
		return types.NewErrBadRequest("refresh is only supported for ECR image pull secrets")
	}

	if !secret.Spec.Enabled {
		return types.NewErrBadRequest("image pull secret is disabled")
	}
	if secret.Annotations == nil {
		secret.Annotations = map[string]string{}
	}
	secret.Annotations[imagepullsecrets.AnnotationECRRefreshRequestedAt] = time.Now().UTC().Format(time.RFC3339Nano)
	if err := req.Update(&secret); err != nil {
		return fmt.Errorf("failed to request ECR refresh: %w", err)
	}

	return req.Write(types.ImagePullSecretRefreshResponse{
		Message: "ECR refresh started",
	})
}

func (h *ImagePullSecretHandler) testECRImagePullSecret(req api.Context, secret v1.ImagePullSecret, image string) (imagepullsecrets.RegistryTestResult, error) {
	if h.runtimeClient == nil {
		return imagepullsecrets.RegistryTestResult{}, types.NewErrHTTP(http.StatusServiceUnavailable, "Kubernetes runtime client is not configured")
	}

	var kubeSecret corev1.Secret
	if err := h.runtimeClient.Get(req.Context(), kclient.ObjectKey{
		Namespace: h.mcpNamespace,
		Name:      secret.Name,
	}, &kubeSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return imagepullsecrets.RegistryTestResult{}, types.NewErrHTTP(http.StatusServiceUnavailable, "generated Kubernetes image pull secret is not ready")
		}
		return imagepullsecrets.RegistryTestResult{}, fmt.Errorf("failed to get Kubernetes image pull secret: %w", err)
	}
	if kubeSecret.Type != corev1.SecretTypeDockerConfigJson {
		return imagepullsecrets.RegistryTestResult{}, types.NewErrBadRequest("generated Kubernetes image pull secret has type %q, expected %q", kubeSecret.Type, corev1.SecretTypeDockerConfigJson)
	}

	result, err := imagepullsecrets.TestDockerConfigJSONCredentials(req.Context(), kubeSecret.Data[corev1.DockerConfigJsonKey], image)
	if err != nil {
		return imagepullsecrets.RegistryTestResult{}, types.NewErrBadRequest("image pull secret test failed: %v", err)
	}
	return result, nil
}

func (h *ImagePullSecretHandler) ensureAvailable() error {
	capability := imagepullsecrets.Availability(mcp.IsKubernetesBackend(h.mcpRuntimeBackend), h.staticSecrets)
	if !capability.Available {
		return types.NewErrBadRequest("managed image pull secrets are unavailable: %s", capability.Reason)
	}
	return nil
}

func (h *ImagePullSecretHandler) convertCapability() types.ImagePullSecretCapability {
	capability := imagepullsecrets.Availability(mcp.IsKubernetesBackend(h.mcpRuntimeBackend), h.staticSecrets)
	reason := capability.Reason
	if capability.Available && strings.TrimSpace(h.issuerURL) == "" {
		reason = "Kubernetes service account issuer URL could not be discovered; enter an issuer URL override to generate the AWS trust policy."
		if h.issuerError != "" {
			reason += " Discovery error: " + h.issuerError
		}
	}
	return types.ImagePullSecretCapability{
		Available: capability.Available,
		Reason:    reason,
		IssuerURL: h.issuerURL,
		Subject:   imagepullsecrets.ECRSubject(h.serviceNamespace, h.serviceAccountName),
		Audience:  imagepullsecrets.DefaultECRAudience,
	}
}

func (h *ImagePullSecretHandler) specFromInput(input types.ImagePullSecretManifest, existing *v1.ImagePullSecret) (v1.ImagePullSecretSpec, error) {
	secretType := input.Type
	if secretType == "" && existing != nil {
		secretType = existing.Spec.Type
	}

	spec := v1.ImagePullSecretSpec{
		Enabled:     input.Enabled,
		Type:        secretType,
		DisplayName: strings.TrimSpace(input.DisplayName),
	}

	switch secretType {
	case types.ImagePullSecretTypeBasic:
		var server, username string
		if input.Basic != nil {
			server = input.Basic.Server
			username = input.Basic.Username
		}
		spec.Basic = &types.BasicImagePullSecretConfig{
			Server:   server,
			Username: username,
		}
	case types.ImagePullSecretTypeECR:
		issuerURL := h.issuerURL
		if existing != nil && existing.Spec.ECR != nil && existing.Spec.ECR.IssuerURL != "" {
			issuerURL = existing.Spec.ECR.IssuerURL
		}

		var ecr types.ECRImagePullSecretConfig
		if input.ECR != nil {
			ecr = *input.ECR
		}
		if ecr.IssuerURL != "" {
			issuerURL = ecr.IssuerURL
		}
		if strings.TrimSpace(issuerURL) == "" {
			return spec, types.NewErrBadRequest("issuerURL is required because Kubernetes service account issuer URL could not be discovered")
		}

		spec.ECR = &types.ECRImagePullSecretConfig{
			RoleARN:         ecr.RoleARN,
			Region:          ecr.Region,
			IssuerURL:       issuerURL,
			Audience:        ecr.Audience,
			RefreshSchedule: ecr.RefreshSchedule,
		}
	default:
		return spec, types.NewErrBadRequest("type must be one of %q or %q", types.ImagePullSecretTypeBasic, types.ImagePullSecretTypeECR)
	}

	validated, err := imagepullsecrets.ValidateSpec(spec)
	if err != nil {
		return spec, types.NewErrBadRequest("invalid image pull secret: %v", err)
	}
	return validated, nil
}

func basicImagePullSecretPassword(input types.ImagePullSecretManifest) string {
	if input.Basic == nil {
		return ""
	}
	return input.Basic.Password
}

func (h *ImagePullSecretHandler) convert(secret v1.ImagePullSecret) types.ImagePullSecret {
	result := types.ImagePullSecret{
		Metadata: MetadataFrom(&secret),
		Manifest: types.ImagePullSecretManifest{
			Enabled:     secret.Spec.Enabled,
			Type:        secret.Spec.Type,
			DisplayName: secret.Spec.DisplayName,
		},
		Status: types.ImagePullSecretStatus{
			LastReconciledTime: metav1Time(secret.Status.LastReconciledTime),
			LastSuccessTime:    metav1Time(secret.Status.LastSuccessTime),
			LastError:          secret.Status.LastError,
			Subject:            secret.Status.Subject,
			TokenExpiresAt:     metav1Time(secret.Status.TokenExpiresAt),
			RegistryEndpoints:  secret.Status.RegistryEndpoints,
		},
	}

	if secret.Spec.Basic != nil {
		result.Manifest.Basic = &types.BasicImagePullSecretConfig{
			Server:   secret.Spec.Basic.Server,
			Username: secret.Spec.Basic.Username,
		}
	}

	if secret.Spec.ECR != nil {
		issuerURL := firstNonEmpty(secret.Status.IssuerURL, secret.Spec.ECR.IssuerURL, h.issuerURL)
		audience := firstNonEmpty(secret.Status.Audience, secret.Spec.ECR.Audience)
		if audience == "" {
			audience = imagepullsecrets.DefaultECRAudience
		}
		if result.Status.Subject == "" {
			result.Status.Subject = imagepullsecrets.ECRSubject(h.serviceNamespace, h.serviceAccountName)
		}
		result.Manifest.ECR = &types.ECRImagePullSecretConfig{
			RoleARN:         secret.Spec.ECR.RoleARN,
			Region:          secret.Spec.ECR.Region,
			IssuerURL:       issuerURL,
			Audience:        audience,
			RefreshSchedule: secret.Spec.ECR.RefreshSchedule,
		}
		result.Status.TrustPolicyJSON = ecrTrustPolicyJSON(result.Manifest.ECR.RoleARN, result.Manifest.ECR.IssuerURL, result.Status.Subject, result.Manifest.ECR.Audience)
		result.Status.ECRPolicyJSON = h.ecrPolicyJSON
		if result.Status.ECRPolicyJSON == "" {
			result.Status.ECRPolicyJSON = defaultECRPolicyJSON
		}
	}

	return result
}

func listPasswordConfigured(req api.Context, secrets []v1.ImagePullSecret) (map[string]bool, error) {
	result := make(map[string]bool)
	needCredentials := false
	for _, secret := range secrets {
		if secret.Spec.Basic != nil {
			needCredentials = true
			result[secret.Name] = false
		}
	}
	if !needCredentials {
		return result, nil
	}

	credentials, err := req.GPTClient.ListCredentials(req.Context(), gptscript.ListCredentialsOptions{
		CredentialContexts: []string{imagepullsecrets.CredentialContext},
	})
	if err != nil {
		return nil, err
	}
	for _, credential := range credentials {
		if _, ok := result[credential.ToolName]; ok {
			_, result[credential.ToolName] = credential.Env[imagepullsecrets.PasswordEnvVar]
		}
	}
	return result, nil
}

func setPasswordConfigured(req api.Context, name string, status *types.ImagePullSecretStatus) error {
	configured, err := passwordConfigured(req, name)
	if err != nil {
		return err
	}
	status.PasswordConfigured = configured
	return nil
}

func storeImagePullSecretPassword(req api.Context, name, password string) error {
	return req.GPTClient.CreateCredential(req.Context(), gptscript.Credential{
		Type:     gptscript.CredentialTypeTool,
		Context:  imagepullsecrets.CredentialContext,
		ToolName: name,
		Env: map[string]string{
			imagepullsecrets.PasswordEnvVar: password,
		},
	})
}

func revealImagePullSecretPassword(req api.Context, name string) (string, error) {
	credential, err := req.GPTClient.RevealCredential(req.Context(), []string{imagepullsecrets.CredentialContext}, name)
	if errors.As(err, &gptscript.ErrNotFound{}) {
		return "", types.NewErrBadRequest("password is not configured")
	}
	if err != nil {
		return "", err
	}
	password := credential.Env[imagepullsecrets.PasswordEnvVar]
	if password == "" {
		return "", types.NewErrBadRequest("password is not configured")
	}
	return password, nil
}

func passwordConfigured(req api.Context, name string) (bool, error) {
	credential, err := req.GPTClient.RevealCredential(req.Context(), []string{imagepullsecrets.CredentialContext}, name)
	if errors.As(err, &gptscript.ErrNotFound{}) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return credential.Env[imagepullsecrets.PasswordEnvVar] != "", nil
}

func deleteImagePullSecretPassword(req api.Context, name string) error {
	err := req.GPTClient.DeleteCredential(req.Context(), imagepullsecrets.CredentialContext, name)
	if errors.As(err, &gptscript.ErrNotFound{}) {
		return nil
	}
	return err
}

func metav1Time(t *metav1.Time) *types.Time {
	if t == nil {
		return nil
	}
	return types.NewTime(t.Time)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func ecrTrustPolicyJSON(roleARN, issuerURL, subject, audience string) string {
	if roleARN == "" || issuerURL == "" || subject == "" || audience == "" {
		return ""
	}

	parts := strings.SplitN(roleARN, ":", 6)
	if len(parts) != 6 {
		return ""
	}
	issuer := strings.TrimPrefix(issuerURL, "https://")
	providerARN := fmt.Sprintf("arn:%s:iam::%s:oidc-provider/%s", parts[1], parts[4], issuer)
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{
				"Effect": "Allow",
				"Principal": map[string]string{
					"Federated": providerARN,
				},
				"Action": "sts:AssumeRoleWithWebIdentity",
				"Condition": map[string]map[string]string{
					"StringEquals": {
						issuer + ":sub": subject,
						issuer + ":aud": audience,
					},
				},
			},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return ""
	}
	return string(data)
}

func buildECRPolicyJSON() string {
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{
			{
				"Effect": "Allow",
				"Action": []string{
					"ecr:GetAuthorizationToken",
				},
				"Resource": "*",
			},
			{
				"Effect": "Allow",
				"Action": []string{
					"ecr:BatchCheckLayerAvailability",
					"ecr:BatchGetImage",
					"ecr:GetDownloadUrlForLayer",
				},
				"Resource": "*",
			},
		},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return ""
	}
	return string(data)
}
