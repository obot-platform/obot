package bedrock

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4signer "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/pkg/system"
)

const (
	AccessKeyIDEnv     = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_ACCESS_KEY_ID"
	SecretAccessKeyEnv = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SECRET_ACCESS_KEY"
	SessionTokenEnv    = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SESSION_TOKEN"
	RegionEnv          = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_REGION"
	SigningServiceEnv  = "OBOT_AMAZON_BEDROCK_MODEL_PROVIDER_SIGNING_SERVICE"
	APIKeyEnv          = "OBOT_AMAZON_BEDROCK_API_KEY_MODEL_PROVIDER_API_KEY"
	APIKeyRegionEnv    = "OBOT_AMAZON_BEDROCK_API_KEY_MODEL_PROVIDER_REGION"

	defaultRegion = "us-east-1"
)

type StaticAuth struct {
	Region          string
	SigningService  string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

func IsProvider(providerName string) bool {
	return providerName == system.AmazonBedrockModelProvider || providerName == system.AmazonBedrockAPIKeyModelProvider
}

func BaseURL(providerName string, credentials map[string]string, dialect nanobottypes.Dialect) (url.URL, error) {
	u, err := RootURL(providerName, credentials)
	if err != nil {
		return url.URL{}, err
	}
	routeDialect, err := RouteDialect(dialect)
	if err != nil {
		return url.URL{}, err
	}
	u.Path = fmt.Sprintf("/%s/v1", routeDialect)
	return u, nil
}

func RootURL(providerName string, credentials map[string]string) (url.URL, error) {
	var region string
	switch providerName {
	case system.AmazonBedrockModelProvider:
		auth, err := StaticAuthFromCredential(credentials)
		if err != nil {
			return url.URL{}, err
		}
		region = auth.Region
	case system.AmazonBedrockAPIKeyModelProvider:
		region = APIKeyRegionFromCredential(credentials)
	default:
		return url.URL{}, fmt.Errorf("unsupported Bedrock model provider %q", providerName)
	}

	return url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("bedrock-mantle.%s.api.aws", region),
		Path:   "/v1",
	}, nil
}

func Transport(providerName string, credentials map[string]string, next http.RoundTripper) (http.RoundTripper, error) {
	switch providerName {
	case system.AmazonBedrockModelProvider:
		auth, err := StaticAuthFromCredential(credentials)
		if err != nil {
			return nil, err
		}
		return SigV4Transport{Auth: auth, Next: next}, nil
	case system.AmazonBedrockAPIKeyModelProvider:
		key := credentials[APIKeyEnv]
		if key == "" {
			return nil, fmt.Errorf("missing %s for Amazon Bedrock API key model provider", APIKeyEnv)
		}
		return APIKeyTransport{Key: key, Next: next}, nil
	default:
		return nil, fmt.Errorf("unsupported Bedrock model provider %q", providerName)
	}
}

func StaticAuthFromCredential(credentials map[string]string) (StaticAuth, error) {
	auth := StaticAuth{
		Region:          credentials[RegionEnv],
		SigningService:  credentials[SigningServiceEnv],
		AccessKeyID:     credentials[AccessKeyIDEnv],
		SecretAccessKey: credentials[SecretAccessKeyEnv],
		SessionToken:    credentials[SessionTokenEnv],
	}
	if auth.Region == "" {
		auth.Region = defaultRegion
	}
	if auth.SigningService == "" {
		auth.SigningService = "bedrock"
	}
	if auth.AccessKeyID == "" {
		return StaticAuth{}, fmt.Errorf("missing %s for Amazon Bedrock model provider", AccessKeyIDEnv)
	}
	if auth.SecretAccessKey == "" {
		return StaticAuth{}, fmt.Errorf("missing %s for Amazon Bedrock model provider", SecretAccessKeyEnv)
	}
	return auth, nil
}

func APIKeyRegionFromCredential(credentials map[string]string) string {
	if region := credentials[APIKeyRegionEnv]; region != "" {
		return region
	}
	return defaultRegion
}

func RouteDialect(dialect nanobottypes.Dialect) (string, error) {
	switch dialect {
	case nanobottypes.DialectAnthropicMessages:
		return "anthropic", nil
	case nanobottypes.DialectOpenAIResponses:
		return "openai", nil
	default:
		return "", fmt.Errorf("unsupported Bedrock model dialect %q", dialect)
	}
}

type APIKeyTransport struct {
	Key  string
	Next http.RoundTripper
}

func (b APIKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Del("X-Api-Key")
	req.Header.Set("Authorization", "Bearer "+b.Key)
	next := b.Next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(req)
}

type SigV4Transport struct {
	Auth StaticAuth
	Next http.RoundTripper
}

func (b SigV4Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := SignRequest(req, b.Auth, time.Now()); err != nil {
		return nil, err
	}
	next := b.Next
	if next == nil {
		next = http.DefaultTransport
	}
	return next.RoundTrip(req)
}

func SignRequest(req *http.Request, auth StaticAuth, signingTime time.Time) error {
	if req.Body == nil {
		req.Body = http.NoBody
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return fmt.Errorf("failed to copy request body for AWS signing: %w", err)
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))

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
		AccessKeyID:     auth.AccessKeyID,
		SecretAccessKey: auth.SecretAccessKey,
		SessionToken:    auth.SessionToken,
	}, req, payloadHash, auth.SigningService, auth.Region, signingTime)
}
