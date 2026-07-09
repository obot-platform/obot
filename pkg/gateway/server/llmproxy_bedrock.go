package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	amazonBedrockModelProvider       = "amazon-bedrock-model-provider"
	amazonBedrockAPIKeyModelProvider = "amazon-bedrock-api-key-model-provider"
)

const (
	bedrockAccessKeyIDEnv     = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID"
	bedrockSecretAccessKeyEnv = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY"
	bedrockSessionTokenEnv    = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SESSION_TOKEN"
	bedrockRegionEnv          = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_REGION"
	bedrockSigningServiceEnv  = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SIGNING_SERVICE"
	bedrockAPIKeyEnv          = "OBOT_AMAZON_BEDROCK_API_KEY_MODEL_PROVIDER_API_KEY"
	bedrockAPIKeyRegionEnv    = "OBOT_AMAZON_BEDROCK_API_KEY_MODEL_PROVIDER_REGION"
)

func (s *Server) newAWSBedrockLLMProviderProxy() *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		backend:                   bedrockMantleProviderBackend{providerName: amazonBedrockModelProvider},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

func (s *Server) newAWSBedrockAPIKeyLLMProviderProxy() *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		backend:                   bedrockMantleProviderBackend{providerName: amazonBedrockAPIKeyModelProvider, apiKey: true},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

type bedrockMantleProviderBackend struct {
	providerName string
	apiKey       bool
}

func (b bedrockMantleProviderBackend) modelProviderName() string {
	return b.providerName
}

func (b bedrockMantleProviderBackend) prepare(req api.Context, l *llmProviderProxy, modelProvider *v1.ModelProvider, body []byte) (*preparedLLMProxyRequest, error) {
	var bodyMap map[string]any
	if err := json.Unmarshal(body, &bodyMap); err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	modelStr, ok := bodyMap["model"].(string)
	if !ok {
		return nil, fmt.Errorf("missing model in body")
	}

	model, err := getModelFromReference(req.Context(), req.Storage, modelProvider.Namespace, modelStr)
	if apierrors.IsNotFound(err) {
		model, err = l.mapHelper.ResolveTargetModel(modelProvider.Name, modelStr)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get model: %w", err)
	}
	if model.Spec.Manifest.ModelProvider != modelProvider.Name {
		return nil, types2.NewErrBadRequest("requested model does not use provider %q", modelProvider.Name)
	}
	hasAccess, err := l.mapHelper.UserHasAccessToModel(req.User, model.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check model permission: %w", err)
	}
	if !hasAccess {
		return nil, types2.NewErrForbidden("user does not have permission to use model %q", modelStr)
	}
	targetModel := model.Spec.Manifest.TargetModel

	bodyMap["model"] = targetModel
	body, err = json.Marshal(bodyMap)
	if err != nil {
		return nil, err
	}
	return &preparedLLMProxyRequest{
		body:              body,
		model:             targetModel,
		modelProvider:     modelProvider.Name,
		tokenUsageTracker: newTokenUsageTracker(*model),
	}, nil
}

func (b bedrockMantleProviderBackend) upstreamURL(prepared *preparedLLMProxyRequest, credEnv map[string]string) (url.URL, error) {
	if b.apiKey {
		return bedrockBaseURL(bedrockAPIKeyRegionFromCredential(credEnv), prepared.model)
	}
	auth, err := bedrockStaticAuthFromCredential(credEnv)
	if err != nil {
		return url.URL{}, err
	}
	return bedrockBaseURL(auth.region, prepared.model)
}

func (b bedrockMantleProviderBackend) transport(_ v1.ModelProvider, credEnv map[string]string) (http.RoundTripper, error) {
	if b.apiKey {
		key := credEnv[bedrockAPIKeyEnv]
		if key == "" {
			return nil, fmt.Errorf("missing %s for Amazon Bedrock API key model provider", bedrockAPIKeyEnv)
		}
		return bedrockAPIKeyTransport{key: key, next: http.DefaultTransport}, nil
	}
	auth, err := bedrockStaticAuthFromCredential(credEnv)
	if err != nil {
		return nil, err
	}
	return bedrockSigV4Transport{auth: auth, next: http.DefaultTransport}, nil
}

func (b bedrockMantleProviderBackend) proxyModelsList(req api.Context, l *llmProviderProxy, _ *v1.ModelProvider, _ map[string]string) (bool, error) {
	if !isBedrockModelsListRequest(req.Request) {
		return false, nil
	}

	models, err := l.mapHelper.GetUserAccessibleProviderModels(req.User, b.modelProviderName())
	if err != nil {
		return true, fmt.Errorf("failed to determine accessible models: %w", err)
	}

	data := make([]map[string]string, 0, len(models))
	for _, model := range models {
		data = append(data, map[string]string{
			"id":     model.Spec.Manifest.TargetModel,
			"object": "model",
		})
	}

	return true, req.Write(map[string]any{
		"object": "list",
		"data":   data,
	})
}

type bedrockStaticAuth struct {
	region          string
	signingService  string
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
}

func bedrockStaticAuthFromCredential(cred map[string]string) (bedrockStaticAuth, error) {
	auth := bedrockStaticAuth{
		region:          cred[bedrockRegionEnv],
		signingService:  cred[bedrockSigningServiceEnv],
		accessKeyID:     cred[bedrockAccessKeyIDEnv],
		secretAccessKey: cred[bedrockSecretAccessKeyEnv],
		sessionToken:    cred[bedrockSessionTokenEnv],
	}
	if auth.region == "" {
		auth.region = "us-east-1"
	}
	if auth.signingService == "" {
		auth.signingService = "bedrock"
	}
	if auth.accessKeyID == "" {
		return bedrockStaticAuth{}, fmt.Errorf("missing %s for Amazon Bedrock model provider", bedrockAccessKeyIDEnv)
	}
	if auth.secretAccessKey == "" {
		return bedrockStaticAuth{}, fmt.Errorf("missing %s for Amazon Bedrock model provider", bedrockSecretAccessKeyEnv)
	}
	return auth, nil
}

func bedrockAPIKeyRegionFromCredential(cred map[string]string) string {
	if region := cred[bedrockAPIKeyRegionEnv]; region != "" {
		return region
	}
	return "us-east-1"
}

type bedrockAPIKeyTransport struct {
	key  string
	next http.RoundTripper
}

func (b bedrockAPIKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Del("X-Api-Key")
	req.Header.Set("Authorization", "Bearer "+b.key)
	next := b.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(req)
}

func bedrockBaseURL(region, model string) (url.URL, error) {
	api := ""
	switch {
	case strings.HasPrefix(model, "anthropic."):
		api = "anthropic"
	case strings.HasPrefix(model, "openai."), strings.HasPrefix(model, "google."):
		api = "openai"
	default:
		return url.URL{}, types2.NewErrBadRequest("unsupported Bedrock model %q", model)
	}
	return url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("bedrock-mantle.%s.api.aws", region),
		Path:   fmt.Sprintf("/%s/v1", api),
	}, nil
}

type bedrockSigV4Transport struct {
	auth bedrockStaticAuth
	next http.RoundTripper
}

func (b bedrockSigV4Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := signBedrockRequest(req, b.auth, time.Now()); err != nil {
		return nil, err
	}
	next := b.next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(req)
}

func signBedrockRequest(req *http.Request, auth bedrockStaticAuth, signingTime time.Time) error {
	if req.Body == nil {
		req.Body = http.NoBody
	}
	body, err := copyBody(&req.Body)
	if err != nil {
		return fmt.Errorf("failed to copy request body for AWS signing: %w", err)
	}
	sum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(sum[:])

	req.Header.Del("Authorization")
	req.Header.Del("X-Api-Key")
	req.Header.Del("Forwarded")
	req.Header.Del("X-Forwarded-For")
	req.Header.Del("X-Forwarded-Host")
	req.Header.Del("X-Forwarded-Proto")
	req.Header.Del("X-Real-Ip")
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	return v4signer.NewSigner().SignHTTP(req.Context(), aws.Credentials{
		AccessKeyID:     auth.accessKeyID,
		SecretAccessKey: auth.secretAccessKey,
		SessionToken:    auth.sessionToken,
	}, req, payloadHash, auth.signingService, auth.region, signingTime)
}

func isBedrockModelsListRequest(req *http.Request) bool {
	if req.Method != http.MethodGet {
		return false
	}
	reqPath := req.PathValue("path")
	reqPath = strings.TrimPrefix(reqPath, "anthropic/v1/")
	reqPath = strings.TrimPrefix(reqPath, "openai/v1/")
	reqPath = strings.TrimPrefix(reqPath, "v1/")
	return reqPath == "models"
}
