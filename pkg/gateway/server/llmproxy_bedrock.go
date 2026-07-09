package server

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	types2 "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

const (
	amazonBedrockModelProvider       = "amazon-bedrock-model-provider"
	amazonBedrockAPIKeyModelProvider = "amazon-bedrock-api-key-model-provider"

	amazonBedrockModelProviderDefaultRegion = "us-east-1"
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

func (s *Server) newAWSBedrockLLMProviderProxy(dialect nanobottypes.Dialect) *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		routeDialect:              dialect,
		backend:                   bedrockMantleProviderBackend{providerName: amazonBedrockModelProvider, dialect: dialect},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

func (s *Server) newAWSBedrockAPIKeyLLMProviderProxy(dialect nanobottypes.Dialect) *llmProviderProxy {
	return &llmProviderProxy{
		dailyUserInputTokenLimit:  s.dailyUserInputTokenLimit,
		dailyUserOutputTokenLimit: s.dailyUserOutputTokenLimit,
		routeDialect:              dialect,
		backend:                   bedrockMantleProviderBackend{providerName: amazonBedrockAPIKeyModelProvider, dialect: dialect, apiKey: true},
		mapHelper:                 s.mapHelper,
		messagePolicyHelper:       s.messagePolicyHelper,
	}
}

type bedrockMantleProviderBackend struct {
	providerName string
	dialect      nanobottypes.Dialect
	apiKey       bool
}

func (b bedrockMantleProviderBackend) modelProviderName() string {
	return b.providerName
}

func (b bedrockMantleProviderBackend) upstreamURL(credEnv map[string]string) (url.URL, error) {
	if b.apiKey {
		return bedrockBaseURL(bedrockAPIKeyRegionFromCredential(credEnv), b.dialect)
	}
	auth, err := bedrockStaticAuthFromCredential(credEnv)
	if err != nil {
		return url.URL{}, err
	}
	return bedrockBaseURL(auth.region, b.dialect)
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
		auth.region = amazonBedrockModelProviderDefaultRegion
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
	return amazonBedrockModelProviderDefaultRegion
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

func bedrockBaseURL(region string, modelDialect nanobottypes.Dialect) (url.URL, error) {
	dialect, err := bedrockRouteDialect(modelDialect)
	if err != nil {
		return url.URL{}, err
	}
	return url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("bedrock-mantle.%s.api.aws", region),
		Path:   fmt.Sprintf("/%s/v1", dialect),
	}, nil
}

func bedrockRouteDialect(dialect nanobottypes.Dialect) (string, error) {
	switch dialect {
	case nanobottypes.DialectAnthropicMessages:
		return "anthropic", nil
	case nanobottypes.DialectOpenAIResponses:
		return "openai", nil
	default:
		return "", types2.NewErrBadRequest("unsupported Bedrock model dialect %q", dialect)
	}
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
