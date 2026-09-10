package mcptester

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	ModelProxyUsageTimeout = 5 * time.Second
)

// ModelProxyUsageError contains only local messages, never upstream bodies or URLs.
type ModelProxyUsageError struct {
	Status     int
	Message    string
	RetryAfter string
}

func (e *ModelProxyUsageError) Error() string {
	return e.Message
}

func NewModelProxyUsageHTTPClient() *http.Client {
	client := NewFallbackHTTPClient()
	client.Timeout = ModelProxyUsageTimeout

	return client
}

// NewModelProxyUsageRequest derives the sibling endpoint from the validated
// Responses URL without losing a deployment prefix or its escaped path.
func NewModelProxyUsageRequest(ctx context.Context, responsesURL *url.URL, licenseKey, fingerprint string) (*http.Request, error) {
	if responsesURL == nil || !safeCredential(licenseKey) || !safeCredential(fingerprint) {
		return nil, errors.New("installation license or machine fingerprint is unavailable")
	}

	endpoint := *responsesURL
	prefix := strings.TrimSuffix(endpoint.Path, "/v1/responses")
	if endpoint.RawPath != "" {
		// The endpoint suffix itself may contain percent escapes. Locate the
		// prefix by decoded bytes so escaped slashes in it remain escaped.
		escaped := endpoint.EscapedPath()
		i := 0

		for range len(prefix) {
			if escaped[i] == '%' {
				i += 3
			} else {
				i++
			}
		}

		endpoint.RawPath = escaped[:i] + "/v1/usage"
	}

	endpoint.Path = prefix + "/v1/usage"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, errors.New("failed to prepare model proxy usage request")
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", types.MCPTesterClientName)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(licenseKey))
	req.Header.Set("X-Obot-Machine-Fingerprint", strings.TrimSpace(fingerprint))

	return req, nil
}

func ReadModelProxyUsage(response *http.Response) (types.ModelProxyUsage, error) {
	failure := &ModelProxyUsageError{Status: http.StatusBadGateway, Message: "The model proxy returned an invalid usage response."}
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			failure.Status = http.StatusServiceUnavailable
			failure.Message = "A valid installation license is required to read model proxy usage."
		case http.StatusTooManyRequests:
			failure.Status = http.StatusTooManyRequests
			failure.Message = "Model proxy license validation is temporarily rate limited."
		case http.StatusServiceUnavailable:
			failure.Status = http.StatusServiceUnavailable
			failure.Message = "Model proxy usage is temporarily unavailable."
		}

		if failure.Status == http.StatusTooManyRequests || failure.Status == http.StatusServiceUnavailable {
			value := response.Header.Get("Retry-After")
			if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
				failure.RetryAfter = strconv.FormatInt(seconds, 10)
			} else if date, err := http.ParseTime(value); err == nil {
				failure.RetryAfter = date.UTC().Format(http.TimeFormat)
			}
		}

		return types.ModelProxyUsage{}, failure
	}

	const maxBody = 64 * 1024

	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return types.ModelProxyUsage{}, failure
	}

	// Require every counter: an incomplete response must not look like zero usage.
	type tokens struct {
		Used *int64 `json:"used"`
		Max  *int64 `json:"max"`
	}

	var snapshot struct {
		Input   tokens    `json:"input"`
		Output  tokens    `json:"output"`
		ResetAt time.Time `json:"reset_at"`
	}

	if err := json.Unmarshal(body, &snapshot); err != nil || snapshot.ResetAt.IsZero() ||
		snapshot.Input.Used == nil || snapshot.Input.Max == nil || snapshot.Output.Used == nil || snapshot.Output.Max == nil {
		return types.ModelProxyUsage{}, failure
	}

	if *snapshot.Input.Used < 0 || *snapshot.Input.Max < 0 || *snapshot.Output.Used < 0 || *snapshot.Output.Max < 0 {
		return types.ModelProxyUsage{}, failure
	}

	return types.ModelProxyUsage{
		Input:   types.ModelProxyTokenUsage{Used: *snapshot.Input.Used, Max: *snapshot.Input.Max},
		Output:  types.ModelProxyTokenUsage{Used: *snapshot.Output.Used, Max: *snapshot.Output.Max},
		ResetAt: snapshot.ResetAt.UTC(),
	}, nil
}
