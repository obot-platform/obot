package imagepullsecrets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"k8s.io/client-go/rest"
)

const issuerDiscoveryTimeout = 5 * time.Second

func DiscoverServiceAccountIssuer(ctx context.Context, restConfig *rest.Config) (string, error) {
	if restConfig == nil || strings.TrimSpace(restConfig.Host) == "" {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(ctx, issuerDiscoveryTimeout)
	defer cancel()

	httpClient, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return "", fmt.Errorf("failed to create Kubernetes discovery client: %w", err)
	}

	endpoint := strings.TrimRight(restConfig.Host, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to discover Kubernetes service account issuer: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("kubernetes issuer discovery returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Issuer string `json:"issuer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("failed to decode Kubernetes issuer discovery response: %w", err)
	}

	return NormalizeIssuerURL(payload.Issuer)
}
