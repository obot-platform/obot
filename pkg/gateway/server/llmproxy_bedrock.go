package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const amazonBedrockModelProvider = "amazon-bedrock-model-provider"

var bedrockOpenAIModels = map[string]bool{
	"openai.gpt-5.4": true,
	"openai.gpt-5.5": true,
}

const (
	bedrockAccessKeyIDEnv     = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID"
	bedrockSecretAccessKeyEnv = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY"
	bedrockSessionTokenEnv    = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SESSION_TOKEN"
	bedrockRegionEnv          = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_REGION"
	bedrockSigningServiceEnv  = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SIGNING_SERVICE"
)

func (s *Server) newAWSBedrockLLMProviderProxy() *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		backend:                   bedrockMantleProviderBackend{},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

type bedrockMantleProviderBackend struct{}

func (b bedrockMantleProviderBackend) modelProviderName() string {
	return amazonBedrockModelProvider
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

	var err error
	model := &v1.Model{}
	if isBedrockOpenAIModel(modelStr) {
		model.Name = modelStr
		model.Spec.Manifest.ModelProvider = amazonBedrockModelProvider
		model.Spec.Manifest.TargetModel = modelStr
		model.Spec.Manifest.Dialect = string(nanobottypes.DialectOpenAIResponses)
	} else {
		model, err = getBedrockModelFromReference(req.Context(), req.Storage, req.Namespace(), modelStr)
		if err != nil {
			return nil, fmt.Errorf("failed to get model: %w", err)
		}
		if model.Spec.Manifest.ModelProvider != modelProvider.Name {
			return nil, types2.NewErrBadRequest("requested model does not use provider %q", amazonBedrockModelProvider)
		}
		hasAccess, err := l.mapHelper.UserHasAccessToModel(req.User, model.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to check model permission: %w", err)
		}
		if !hasAccess {
			return nil, types2.NewErrForbidden("user does not have permission to use model %q", modelStr)
		}
	}

	targetModel := bedrockMantleModelName(model.Spec.Manifest.TargetModel)
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
	auth, err := bedrockStaticAuthFromCredential(credEnv)
	if err != nil {
		return url.URL{}, err
	}
	return bedrockBaseURL(auth.region, prepared.model), nil
}

func (b bedrockMantleProviderBackend) transport(_ v1.ModelProvider, credEnv map[string]string) (http.RoundTripper, error) {
	auth, err := bedrockStaticAuthFromCredential(credEnv)
	if err != nil {
		return nil, err
	}
	return bedrockSigV4Transport{auth: auth, next: http.DefaultTransport}, nil
}

func (b bedrockMantleProviderBackend) proxyModelsList(req api.Context, l *llmProviderProxy, _ *v1.ModelProvider, credEnv map[string]string) (bool, error) {
	if !isBedrockModelsListRequest(req.Request) {
		return false, nil
	}
	auth, err := bedrockStaticAuthFromCredential(credEnv)
	if err != nil {
		return true, err
	}
	(&httputil.ReverseProxy{
		Director:  bedrockModelsListTransformRequest(auth.region),
		Transport: bedrockSigV4Transport{auth: auth, next: http.DefaultTransport},
		ModifyResponse: func(resp *http.Response) error {
			return filterBedrockModelListResponse(resp, l.mapHelper, req.User)
		},
	}).ServeHTTP(req.ResponseWriter, req.Request)
	return true, nil
}

func getBedrockModelFromReference(ctx context.Context, client kclient.Client, namespace, modelReference string) (*v1.Model, error) {
	m, err := getModelFromReference(ctx, client, namespace, modelReference)
	if err == nil || !apierrors.IsNotFound(err) {
		return m, err
	}

	var models v1.ModelList
	if err := client.List(ctx, &models, &kclient.ListOptions{Namespace: namespace}); err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}
	for _, model := range models.Items {
		if model.Spec.Manifest.ModelProvider == amazonBedrockModelProvider && model.Spec.Manifest.Active && slices.Contains(bedrockModelNames(model.Spec.Manifest.TargetModel), modelReference) {
			return &model, nil
		}
	}

	return nil, apierrors.NewNotFound(schema.GroupResource{Group: v1.SchemeGroupVersion.Group, Resource: "model"}, modelReference)
}

func bedrockModelNames(model string) []string {
	seen := map[string]bool{model: true}
	if isBedrockOpenAIModel(model) {
		return []string{model}
	}
	if mantleName := bedrockMantleModelName(model); strings.HasPrefix(mantleName, "anthropic.") {
		seen[mantleName] = true
		for _, prefix := range []string{"us.", "eu.", "apac.", "us-gov."} {
			seen[prefix+mantleName] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	return names
}

func bedrockMantleModelName(model string) string {
	if isBedrockOpenAIModel(model) {
		return model
	}
	model = strings.TrimPrefix(model, "us.")
	model = strings.TrimPrefix(model, "eu.")
	model = strings.TrimPrefix(model, "apac.")
	model = strings.TrimPrefix(model, "us-gov.")
	if i := strings.LastIndex(model, "-"); i > 0 && strings.HasSuffix(model, ":0") {
		version := model[i+1:]
		if strings.HasPrefix(version, "v") {
			model = model[:i]
		}
	}
	if i := strings.LastIndex(model, "-"); i > 0 && isYYYYMMDD(model[i+1:]) {
		model = model[:i]
	}
	return model
}

func isYYYYMMDD(s string) bool {
	if len(s) != 8 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

func bedrockBaseURL(region, model string) url.URL {
	api := "anthropic"
	if isBedrockOpenAIModel(model) {
		api = "openai"
	}
	return url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("bedrock-mantle.%s.api.aws", region),
		Path:   fmt.Sprintf("/%s/v1", api),
	}
}

func bedrockModelsListTransformRequest(region string) func(req *http.Request) {
	u := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("bedrock-mantle.%s.api.aws", region),
		Path:   "/v1/models",
	}
	return func(req *http.Request) {
		reqURL := u
		req.URL = &reqURL
		req.Host = reqURL.Host
		req.Header.Del("Accept-Encoding")
		req.Header.Del(internalRequestTypeHeader)
	}
}

func isBedrockOpenAIModel(model string) bool {
	return bedrockOpenAIModels[model]
}

func filterBedrockModelListResponse(resp *http.Response, mapHelper *modelaccesspolicy.Helper, user kuser.Info) error {
	if resp.StatusCode >= http.StatusBadRequest {
		body, err := copyBody(&resp.Body)
		if err != nil {
			return fmt.Errorf("failed to copy AWS Bedrock models error response body: %w", err)
		}
		log.Infof("AWS Bedrock LLM proxy models error response: status=%d body=%s", resp.StatusCode, string(body))
		return nil
	}

	allowedTargetModels, allowAllModels, err := mapHelper.GetUserAllowedTargetModels(user, amazonBedrockModelProvider)
	if err != nil {
		return fmt.Errorf("failed to determine accessible models: %w", err)
	}
	if allowAllModels {
		return nil
	}

	allowed := make(map[string]bool, len(allowedTargetModels)*3)
	for model := range allowedTargetModels {
		for _, name := range bedrockModelNames(model) {
			allowed[name] = true
		}
	}
	for model := range bedrockOpenAIModels {
		allowed[model] = true
	}
	return filterModelListResponse(resp, allowed, false)
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
	log.Debugf(
		"AWS Bedrock LLM proxy signing request: service=%s region=%s host=%s path=%s access_key_suffix=%s session_token=%t payload_sha256=%s",
		auth.signingService,
		auth.region,
		req.URL.Host,
		req.URL.Path,
		accessKeySuffix(auth.accessKeyID),
		auth.sessionToken != "",
		payloadHash,
	)

	return v4signer.NewSigner().SignHTTP(req.Context(), aws.Credentials{
		AccessKeyID:     auth.accessKeyID,
		SecretAccessKey: auth.secretAccessKey,
		SessionToken:    auth.sessionToken,
	}, req, payloadHash, auth.signingService, auth.region, signingTime)
}

func accessKeySuffix(accessKeyID string) string {
	if len(accessKeyID) <= 4 {
		return accessKeyID
	}
	return accessKeyID[len(accessKeyID)-4:]
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
