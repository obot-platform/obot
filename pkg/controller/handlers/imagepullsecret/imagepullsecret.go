package imagepullsecret

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/adhocore/gronx"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/imagepullsecrets"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	annotationECRConfigHash = "obot.ai/ecr-config-hash"
)

type Handler struct {
	gptClient          *gptscript.GPTScript
	runtimeClient      kclient.Client
	mcpRuntimeBackend  string
	mcpNamespace       string
	serviceNamespace   string
	serviceAccountName string
	staticSecrets      []string
	issuerURL          string
	now                func() time.Time
}

func New(gptClient *gptscript.GPTScript, runtimeClient kclient.Client, mcpRuntimeBackend, mcpNamespace, serviceNamespace, serviceAccountName string, staticSecrets []string, issuerURL string) *Handler {
	return &Handler{
		gptClient:          gptClient,
		runtimeClient:      runtimeClient,
		mcpRuntimeBackend:  mcpRuntimeBackend,
		mcpNamespace:       mcpNamespace,
		serviceNamespace:   firstNonEmpty(serviceNamespace, mcpNamespace),
		serviceAccountName: strings.TrimSpace(serviceAccountName),
		staticSecrets:      staticSecrets,
		issuerURL:          issuerURL,
		now:                time.Now,
	}
}

func (h *Handler) Reconcile(req router.Request, resp router.Response) error {
	secret := req.Object.(*v1.ImagePullSecret)
	if err := h.reconcile(req, resp, secret); err != nil {
		statusErr := h.updateStatus(req, secret, func(status *v1.ImagePullSecretStatus) {
			status.LastError = sanitizeError(err)
		})
		return errors.Join(err, statusErr)
	}
	return nil
}

func (h *Handler) Cleanup(req router.Request, _ router.Response) error {
	secret := req.Object.(*v1.ImagePullSecret)

	if err := h.deleteK8sSecret(req.Ctx, secret); err != nil {
		return err
	}
	if err := h.gptClient.DeleteCredential(req.Ctx, imagepullsecrets.CredentialContext, secret.Name); err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return err
	}

	return nil
}

func (h *Handler) reconcile(req router.Request, resp router.Response, secret *v1.ImagePullSecret) error {
	capability := imagepullsecrets.Availability(mcp.IsKubernetesBackend(h.mcpRuntimeBackend), h.staticSecrets)
	if !capability.Available {
		return h.updateStatus(req, secret, func(status *v1.ImagePullSecretStatus) {
			status.LastError = capability.Reason
		})
	}

	validated, err := imagepullsecrets.ValidateSpec(secret.Spec)
	if err != nil {
		return fmt.Errorf("invalid image pull secret spec: %w", err)
	}
	secret.Spec = validated

	if !secret.Spec.Enabled {
		if err := h.deleteK8sSecret(req.Ctx, secret); err != nil {
			return err
		}
		return h.updateStatus(req, secret, func(status *v1.ImagePullSecretStatus) {
			status.LastError = ""
		})
	}

	switch secret.Spec.Type {
	case apitypes.ImagePullSecretTypeBasic:
		return h.reconcileBasic(req, secret)
	case apitypes.ImagePullSecretTypeECR:
		return h.reconcileECR(req, resp, secret)
	default:
		return fmt.Errorf("unsupported image pull secret type %q", secret.Spec.Type)
	}
}

func (h *Handler) reconcileBasic(req router.Request, secret *v1.ImagePullSecret) error {
	password, err := h.revealPassword(req.Ctx, secret.Name)
	if err != nil {
		return err
	}
	dockerConfigJSON, err := imagepullsecrets.BuildDockerConfigJSON(secret.Spec.Basic.Server, secret.Spec.Basic.Username, password)
	if err != nil {
		return err
	}
	if err := h.writeDockerConfigSecret(req.Ctx, secret, dockerConfigJSON, ""); err != nil {
		return err
	}
	return h.updateStatus(req, secret, func(status *v1.ImagePullSecretStatus) {
		status.LastSuccessTime = new(metav1.Now())
		status.LastError = ""
	})
}

func (h *Handler) reconcileECR(req router.Request, resp router.Response, secret *v1.ImagePullSecret) error {
	configChanged, err := h.ecrConfigChanged(req.Ctx, secret)
	if err != nil {
		return err
	}
	if h.shouldRefreshECR(secret, configChanged) {
		return h.refreshECR(req, resp, secret)
	}

	h.scheduleNextRefresh(resp, secret)
	return h.updateStatus(req, secret, func(status *v1.ImagePullSecretStatus) {
		h.populateECRComputedStatus(secret, status)
		status.LastError = ""
	})
}

func (h *Handler) revealPassword(ctx context.Context, name string) (string, error) {
	credential, err := h.gptClient.RevealCredential(ctx, []string{imagepullsecrets.CredentialContext}, name)
	if errors.As(err, &gptscript.ErrNotFound{}) {
		return "", errors.New("password is not configured")
	}
	if err != nil {
		return "", fmt.Errorf("failed to reveal password credential: %w", err)
	}
	password := credential.Env[imagepullsecrets.PasswordEnvVar]
	if password == "" {
		return "", errors.New("password is not configured")
	}
	return password, nil
}

func (h *Handler) ecrConfigChanged(ctx context.Context, imagePullSecret *v1.ImagePullSecret) (bool, error) {
	existing := &corev1.Secret{}
	if err := h.runtimeClient.Get(ctx, types.NamespacedName{Namespace: h.mcpNamespace, Name: imagePullSecret.Name}, existing); err != nil {
		if apierrors.IsNotFound(err) {
			return true, nil
		}
		return false, fmt.Errorf("failed to read Kubernetes image pull secret %s/%s: %w", h.mcpNamespace, imagePullSecret.Name, err)
	}
	return existing.Annotations[annotationECRConfigHash] != ecrConfigHash(imagePullSecret), nil
}

func (h *Handler) shouldRefreshECR(secret *v1.ImagePullSecret, configChanged bool) bool {
	if configChanged || secret.Status.LastSuccessTime == nil {
		return true
	}
	if requestedAt, ok := ecrRefreshRequestedAt(secret); ok {
		if secret.Status.LastReconciledTime == nil || requestedAt.After(secret.Status.LastReconciledTime.Time) {
			return true
		}
	}
	if secret.Status.TokenExpiresAt != nil && !secret.Status.TokenExpiresAt.After(h.now().Add(time.Hour)) {
		return true
	}
	next, err := gronx.NextTickAfter(secret.Spec.ECR.RefreshSchedule, secret.Status.LastSuccessTime.Time, false)
	return err == nil && !next.After(h.now())
}

func (h *Handler) scheduleNextRefresh(resp router.Response, secret *v1.ImagePullSecret) {
	if secret.Status.LastSuccessTime == nil {
		return
	}
	next, err := gronx.NextTickAfter(secret.Spec.ECR.RefreshSchedule, secret.Status.LastSuccessTime.Time, false)
	if err != nil {
		return
	}
	if until := next.Sub(h.now()); until > 0 && until < 10*time.Hour { // controller handlers are automatically triggered every 10 hours
		resp.RetryAfter(until)
	}
}

func (h *Handler) refreshECR(req router.Request, resp router.Response, secret *v1.ImagePullSecret) error {
	token, err := h.createECRServiceAccountToken(req.Ctx, secret.Spec.ECR)
	if err != nil {
		return err
	}
	ecrClient, err := h.ecrClient(req.Ctx, secret.Spec.ECR, token)
	if err != nil {
		return err
	}

	attemptedAt := metav1.NewTime(h.now().UTC())
	result, err := imagepullsecrets.FetchECRDockerConfig(req.Ctx, ecrClient)
	if err != nil {
		statusErr := h.updateStatus(req, secret, func(status *v1.ImagePullSecretStatus) {
			h.populateECRComputedStatus(secret, status)
			status.LastError = fmt.Sprintf("failed to get ECR authorization token: %s", sanitizeError(err))
		})
		if statusErr != nil {
			return errors.Join(err, statusErr)
		}
		return fmt.Errorf("failed to get ECR authorization token: %w", err)
	}

	if err := h.writeDockerConfigSecret(req.Ctx, secret, result.DockerConfigJSON, ecrConfigHash(secret)); err != nil {
		statusErr := h.updateStatus(req, secret, func(status *v1.ImagePullSecretStatus) {
			h.populateECRComputedStatus(secret, status)
			status.LastError = fmt.Sprintf("failed to write image pull secret: %s", sanitizeError(err))
		})
		if statusErr != nil {
			return errors.Join(err, statusErr)
		}
		return err
	}

	var tokenExpiresAt *metav1.Time
	if result.TokenExpiresAt != nil {
		value := metav1.NewTime(result.TokenExpiresAt.UTC())
		tokenExpiresAt = &value
	}
	if err := h.updateStatus(req, secret, func(status *v1.ImagePullSecretStatus) {
		h.populateECRComputedStatus(secret, status)
		status.LastSuccessTime = &attemptedAt
		status.TokenExpiresAt = tokenExpiresAt
		status.RegistryEndpoints = result.RegistryEndpoints
		status.LastError = ""
	}); err != nil {
		return err
	}
	h.scheduleNextRefresh(resp, secret)
	return nil
}

func (h *Handler) createECRServiceAccountToken(ctx context.Context, ecrSpec *apitypes.ECRImagePullSecretConfig) (string, error) {
	expirationSeconds := int64(3600)

	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      h.serviceAccountName,
			Namespace: h.serviceNamespace,
		},
	}
	tokenRequest := &authenticationv1.TokenRequest{
		Spec: authenticationv1.TokenRequestSpec{
			Audiences:         []string{ecrSpec.Audience},
			ExpirationSeconds: &expirationSeconds,
		},
	}
	if err := h.runtimeClient.SubResource("token").Create(ctx, serviceAccount, tokenRequest); err != nil {
		return "", fmt.Errorf("failed to create ECR service account token: %w", err)
	}
	return tokenRequest.Status.Token, nil
}

func (h *Handler) ecrClient(ctx context.Context, ecrSpec *apitypes.ECRImagePullSecretConfig, token string) (imagepullsecrets.ECRAuthorizationClient, error) {
	stsConfig, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(ecrSpec.Region),
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	stsClient := sts.NewFromConfig(stsConfig)
	sessionName := "obot-" + h.serviceAccountName
	assumed, err := stsClient.AssumeRoleWithWebIdentity(ctx, &sts.AssumeRoleWithWebIdentityInput{
		RoleArn:          aws.String(ecrSpec.RoleARN),
		RoleSessionName:  aws.String(sessionName),
		WebIdentityToken: aws.String(token),
		DurationSeconds:  aws.Int32(3600),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to assume ECR role: %w", err)
	}
	if assumed.Credentials == nil {
		return nil, errors.New("failed to assume ECR role: AWS returned no credentials")
	}

	ecrConfig, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(ecrSpec.Region),
		config.WithCredentialsProvider(staticCredentialsProvider(*assumed.Credentials)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load ECR AWS config: %w", err)
	}
	return ecr.NewFromConfig(ecrConfig), nil
}

func staticCredentialsProvider(creds ststypes.Credentials) aws.CredentialsProvider {
	return credentials.NewStaticCredentialsProvider(aws.ToString(creds.AccessKeyId), aws.ToString(creds.SecretAccessKey), aws.ToString(creds.SessionToken))
}

func (h *Handler) writeDockerConfigSecret(ctx context.Context, imagePullSecret *v1.ImagePullSecret, dockerConfigJSON []byte, ecrConfigHash string) error {
	apply := func(secret *corev1.Secret) {
		secret.Labels = mergeLabels(secret.Labels, managedLabels(imagePullSecret.Name))
		secret.Type = corev1.SecretTypeDockerConfigJson
		secret.Data = map[string][]byte{
			corev1.DockerConfigJsonKey: dockerConfigJSON,
		}
		if ecrConfigHash != "" {
			if secret.Annotations == nil {
				secret.Annotations = map[string]string{}
			}
			secret.Annotations[annotationECRConfigHash] = ecrConfigHash
		}
	}

	existing := &corev1.Secret{}
	err := h.runtimeClient.Get(ctx, types.NamespacedName{Namespace: h.mcpNamespace, Name: imagePullSecret.Name}, existing)
	if apierrors.IsNotFound(err) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      imagePullSecret.Name,
				Namespace: h.mcpNamespace,
			},
		}
		apply(secret)
		return h.runtimeClient.Create(ctx, secret)
	}
	if err != nil {
		return fmt.Errorf("failed to read Kubernetes image pull secret %s/%s: %w", h.mcpNamespace, imagePullSecret.Name, err)
	}

	updated := existing.DeepCopy()
	apply(updated)
	if equality.Semantic.DeepEqual(existing, updated) {
		return nil
	}
	return h.runtimeClient.Update(ctx, updated)
}

func ecrRefreshRequestedAt(secret *v1.ImagePullSecret) (time.Time, bool) {
	value := strings.TrimSpace(secret.Annotations[imagepullsecrets.AnnotationECRRefreshRequestedAt])
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func (h *Handler) deleteK8sSecret(ctx context.Context, secret *v1.ImagePullSecret) error {
	if !mcp.IsKubernetesBackend(h.mcpRuntimeBackend) {
		return nil
	}
	if strings.TrimSpace(secret.Name) == "" {
		return nil
	}

	k8sSecret := &corev1.Secret{}
	if err := h.runtimeClient.Get(ctx, types.NamespacedName{Namespace: h.mcpNamespace, Name: secret.Name}, k8sSecret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to read Kubernetes image pull secret %s/%s for deletion: %w", h.mcpNamespace, secret.Name, err)
	}
	if !isManagedByImagePullSecret(k8sSecret, secret.Name) {
		return nil
	}
	if err := h.runtimeClient.Delete(ctx, k8sSecret); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Kubernetes image pull secret %s/%s: %w", h.mcpNamespace, secret.Name, err)
	}
	return nil
}

func (h *Handler) populateECRComputedStatus(secret *v1.ImagePullSecret, status *v1.ImagePullSecretStatus) {
	status.IssuerURL = firstNonEmpty(secret.Spec.ECR.IssuerURL, h.issuerURL)
	status.Audience = secret.Spec.ECR.Audience
	if status.Audience == "" {
		status.Audience = imagepullsecrets.DefaultECRAudience
	}
	status.Subject = imagepullsecrets.ECRSubject(h.serviceNamespace, h.serviceAccountName)
}

func (h *Handler) updateStatus(req router.Request, secret *v1.ImagePullSecret, mutate func(*v1.ImagePullSecretStatus)) error {
	previous := secret.Status.DeepCopy()
	next := secret.Status.DeepCopy()
	mutate(next)

	if equality.Semantic.DeepEqual(previous, next) {
		return nil
	}

	now := metav1.Now()
	next.LastReconciledTime = &now
	secret.Status = *next
	return req.Client.Status().Update(req.Ctx, secret)
}

func mergeLabels(existing, desired map[string]string) map[string]string {
	labels := make(map[string]string, len(existing)+len(desired))
	maps.Copy(labels, existing)
	maps.Copy(labels, desired)
	return labels
}

func ecrConfigHash(secret *v1.ImagePullSecret) string {
	data, _ := json.Marshal(struct {
		ECR *apitypes.ECRImagePullSecretConfig `json:"ecr,omitempty"`
	}{
		ECR: secret.Spec.ECR,
	})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func managedLabels(imagePullSecretName string) map[string]string {
	return map[string]string{
		imagepullsecrets.LabelManagedBy:       imagepullsecrets.LabelManagedByValue,
		imagepullsecrets.LabelImagePullSecret: imagePullSecretName,
	}
}

func isManagedByImagePullSecret(obj kclient.Object, imagePullSecretName string) bool {
	labels := obj.GetLabels()
	return labels[imagepullsecrets.LabelManagedBy] == imagepullsecrets.LabelManagedByValue &&
		labels[imagepullsecrets.LabelImagePullSecret] == imagePullSecretName
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.TrimSpace(err.Error())
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 512 {
		message = message[:512]
	}
	return message
}
