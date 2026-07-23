package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	communityLicenseEndpoint         = "community-license"
	communityLicenseRequestTimeout   = 10 * time.Second
	communityLicenseTokenLifetime    = 5 * time.Minute
	maxCommunityLicenseResponseBytes = 64 * 1024
)

var (
	ErrCommunityLicenseRequest  = errors.New("community license request failed")
	ErrCommunityLicenseResponse = errors.New("community license response was invalid")
)

type CommunityLicenseRequest struct {
	Name           string `json:"name"`
	Email          string `json:"email"`
	Company        string `json:"company,omitempty"`
	InstallationID string `json:"installationId"`
}

type CommunityLicenseIssuer interface {
	Issue(context.Context, CommunityLicenseRequest) (string, error)
}

type CommunityLicenseClient struct {
	gatewayClient propertyClient
	httpClient    *http.Client
	issueURL      string
}

type communityLicenseResponse struct {
	LicenseKey string `json:"licenseKey"`
}

type sanitizedError struct {
	public error
	cause  error
}

func (e *sanitizedError) Error() string {
	return e.public.Error()
}

func (e *sanitizedError) Unwrap() []error {
	if e.cause == nil {
		return []error{e.public}
	}
	return []error{e.public, e.cause}
}

func NewCommunityLicenseIssuer(gatewayClient propertyClient, baseURL string, httpClient *http.Client) *CommunityLicenseClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &CommunityLicenseClient{
		gatewayClient: gatewayClient,
		httpClient:    httpClient,
		issueURL:      EndpointURL(baseURL, communityLicenseEndpoint),
	}
}

func (c *CommunityLicenseClient) Issue(ctx context.Context, request CommunityLicenseRequest) (string, error) {
	installationID, err := GetInstallationID(ctx, c.gatewayClient)
	if err != nil {
		return "", requestError(err)
	}

	request.Name = strings.TrimSpace(request.Name)
	request.Email = strings.TrimSpace(request.Email)
	request.Company = strings.TrimSpace(request.Company)
	request.InstallationID = installationID

	body, err := json.Marshal(request)
	if err != nil {
		return "", requestError(err)
	}

	requestContext, cancel := context.WithTimeout(ctx, communityLicenseRequestTimeout)
	defer cancel()

	httpRequest, err := http.NewRequestWithContext(requestContext, http.MethodPost, c.issueURL, bytes.NewReader(body))
	if err != nil {
		return "", requestError(err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		return "", requestError(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", requestError(fmt.Errorf("non-OK status code: %d", response.StatusCode))
	}

	limitedBody := io.LimitReader(response.Body, maxCommunityLicenseResponseBytes+1)
	responseBody, err := io.ReadAll(limitedBody)
	if err != nil {
		return "", responseError(err)
	}
	if len(responseBody) > maxCommunityLicenseResponseBytes {
		return "", responseError(fmt.Errorf("response body too large: %d", len(responseBody)))
	}

	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.DisallowUnknownFields()
	var result communityLicenseResponse
	if err := decoder.Decode(&result); err != nil {
		return "", responseError(err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return "", responseError(err)
	}

	result.LicenseKey = strings.TrimSpace(result.LicenseKey)
	if result.LicenseKey == "" {
		return "", responseError(fmt.Errorf("license key is empty"))
	}
	return result.LicenseKey, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}

func requestError(cause error) error {
	return &sanitizedError{public: ErrCommunityLicenseRequest, cause: cause}
}

func responseError(cause error) error {
	return &sanitizedError{public: ErrCommunityLicenseResponse, cause: cause}
}
