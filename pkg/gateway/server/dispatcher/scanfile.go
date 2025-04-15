package dispatcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type fileScanRequest struct {
	Contents []byte `json:"contents"`
}

func (d *Dispatcher) ScanFile(ctx context.Context, contents []byte) (bool, error) {
	config, err := d.gatewayClient.GetVirusScannerConfig(ctx)
	if err != nil || config.ProviderName == "" || config.ProviderNamespace == "" {
		// If the provider name or namespace is not set, then virus scanning is essentially disabled.
		return true, err
	}

	u, err := d.urlForFileScannerProvider(ctx, config.ProviderNamespace, config.ProviderName)
	if err != nil {
		return false, err
	}

	body := fileScanRequest{
		Contents: contents,
	}

	b, err := json.Marshal(body)
	if err != nil {
		return false, fmt.Errorf("failed to marshal body: %w", err)
	}

	u.Path = "/file"

	req, err := http.NewRequestWithContext(ctx, "POST", u.String(), bytes.NewReader(b))
	if err != nil {
		return false, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		b, _ = io.ReadAll(resp.Body)
		return true, fmt.Errorf("file is potentially malicious: %s", b)
	} else if resp.StatusCode != http.StatusOK {
		b, _ = io.ReadAll(resp.Body)
		return false, fmt.Errorf("failed to scan file: %s, %s", resp.Status, b)
	}

	return true, nil
}
