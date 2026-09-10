package mcptester

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	DefaultModelProxyURL = "https://model-service.obot.ai"
	FallbackModel        = "gpt-5.6-luna"
	FallbackMaxBodyBytes = 2 << 20
	FallbackTimeout      = 10 * time.Minute
)

type ProviderConfigurationResolver interface {
	HasModelProvider(context.Context) (bool, error)
}

type LicenseSource interface {
	LicenseKey(context.Context) (string, error)
	MachineFingerprint() string
}

type ModelProxySettingsReader interface {
	ModelProxyEnabled(context.Context) (bool, error)
}

// ParseModelProxyURL validates local configuration without contacting the proxy.
// An empty value disables fallback; callers supply the default only when unset.
func ParseModelProxyURL(value string, development bool) (*url.URL, error) {
	if value == "" {
		return nil, nil
	}

	parsed, err := url.Parse(value)
	if err != nil || !validServiceHost(parsed) || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.Contains(value, "#") ||
		(parsed.Scheme != "https" && (!development || parsed.Scheme != "http")) {
		return nil, errors.New("OBOT_SERVER_MODEL_PROXY_URL must be an absolute HTTPS service URL without credentials, query, or fragment")
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawPath = strings.TrimRight(parsed.RawPath, "/")
	if !strings.HasSuffix(parsed.Path, "/v1/responses") {
		parsed.Path += "/v1/responses"
		if parsed.RawPath != "" {
			parsed.RawPath += "/v1/responses"
		}
	}

	return parsed, nil
}

func validServiceHost(parsed *url.URL) bool {
	if parsed == nil || parsed.Opaque != "" || parsed.Hostname() == "" {
		return false
	}

	host := parsed.Hostname()
	if net.ParseIP(host) == nil {
		if strings.ContainsAny(parsed.Host, "[]:") && parsed.Port() == "" {
			return false
		}

		if strings.HasPrefix(parsed.Host, "[") {
			return false
		}

		for _, char := range host {
			valid := char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '-'
			if !valid {
				return false
			}
		}
	}

	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		return err == nil && number > 0 && number <= 65535
	}

	return !strings.HasSuffix(parsed.Host, ":")
}

func BuildFallbackRequest(request types.MCPTesterChatRequest, instruction string) ([]byte, error) {
	if err := ValidateChatRequest(request); err != nil {
		return nil, err
	}

	payload := buildResponsesRequest(request, FallbackModel, instruction)
	payload["reasoning"] = map[string]string{"effort": "high"}
	payload["store"] = false
	payload["max_output_tokens"] = 16384

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	if len(body) > FallbackMaxBodyBytes {
		return nil, errors.New("model request exceeds 2 MiB")
	}

	return body, nil
}

// NewFallbackRequest deliberately has no inbound-header argument. Only the
// installation credential and sanitized server descriptor leave Obot.
func NewFallbackRequest(ctx context.Context, endpoint *url.URL, body []byte, licenseKey, fingerprint string, descriptor Descriptor) (*http.Request, error) {
	if endpoint == nil || len(body) > FallbackMaxBodyBytes {
		return nil, errors.New("invalid model proxy request")
	}

	if !safeCredential(licenseKey) || !safeCredential(fingerprint) {
		return nil, errors.New("installation license or machine fingerprint is unavailable")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("failed to prepare model proxy request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", types.MCPTesterClientName)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(licenseKey))
	req.Header.Set("X-Obot-Machine-Fingerprint", strings.TrimSpace(fingerprint))
	req.Header.Set(descriptor.Header, descriptor.Value)

	// No replay body: generation attempts must not be automatically retried.
	req.GetBody = nil

	return req, nil
}

func safeCredential(value string) bool {
	if strings.TrimSpace(value) == "" {
		return false
	}

	for _, char := range value {
		if char < 32 || char == 127 {
			return false
		}
	}

	return true
}

func NewFallbackHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = time.Minute

	return &http.Client{
		Transport:     transport,
		Timeout:       FallbackTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
