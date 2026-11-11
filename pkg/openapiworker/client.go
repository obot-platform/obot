package openapiworker

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type ClientConfig struct {
	BaseURL         string
	DevMode         bool
	ServerCAFile    string
	ClientCertFile  string
	ClientKeyFile   string
	BearerTokenFile string
	BearerToken     string
}

type Client struct {
	baseURL     string
	bearerToken string
	httpClient  *http.Client
}

func NewClient(config ClientConfig) (*Client, error) {
	baseURL := strings.TrimSuffix(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		return nil, NewError(ErrorCodeWorkerUnavailable, "OpenAPI Worker URL is required")
	}
	if !strings.HasPrefix(baseURL, "https://") && !(config.DevMode && strings.HasPrefix(baseURL, "http://127.0.0.1:") || config.DevMode && strings.HasPrefix(baseURL, "http://localhost:")) {
		return nil, NewError(ErrorCodeWorkerUnavailable, "OpenAPI Worker URL must use production HTTPS or explicit local HTTP")
	}
	if !config.DevMode && (config.ServerCAFile == "" || config.ClientCertFile == "" || config.ClientKeyFile == "") {
		return nil, NewError(ErrorCodeWorkerUnavailable, "production OpenAPI Worker transport requires server CA and client mTLS materials")
	}

	transport := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        16,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
	}
	if strings.HasPrefix(baseURL, "https://") {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		if config.ServerCAFile != "" {
			caPEM, err := os.ReadFile(config.ServerCAFile)
			if err != nil {
				return nil, NewError(ErrorCodeWorkerUnavailable, "cannot read OpenAPI Worker server CA")
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caPEM) {
				return nil, NewError(ErrorCodeWorkerUnavailable, "invalid OpenAPI Worker server CA")
			}
			tlsConfig.RootCAs = pool
		}
		if config.ClientCertFile != "" || config.ClientKeyFile != "" {
			if config.ClientCertFile == "" || config.ClientKeyFile == "" {
				return nil, NewError(ErrorCodeWorkerUnavailable, "OpenAPI Worker client certificate and key must be configured together")
			}
			certificate, err := tls.LoadX509KeyPair(config.ClientCertFile, config.ClientKeyFile)
			if err != nil {
				return nil, NewError(ErrorCodeWorkerUnavailable, "cannot load OpenAPI Worker client mTLS materials")
			}
			tlsConfig.Certificates = []tls.Certificate{certificate}
		}
		transport.TLSClientConfig = tlsConfig
	}

	bearerToken := strings.TrimSpace(config.BearerToken)
	if bearerToken == "" && config.BearerTokenFile != "" {
		data, err := os.ReadFile(config.BearerTokenFile)
		if err != nil {
			return nil, NewError(ErrorCodeWorkerUnavailable, "cannot read OpenAPI Worker bearer token")
		}
		bearerToken = strings.TrimSpace(string(data))
	}

	return &Client{
		baseURL:     baseURL,
		bearerToken: bearerToken,
		httpClient: &http.Client{
			Transport:     transport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *Client) Ready(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var response struct {
		Status string `json:"status"`
	}
	if err := c.getJSON(ctx, "/readyz", &response); err != nil {
		return err
	}
	if response.Status != "ready" && response.Status != "ok" {
		return NewError(ErrorCodeWorkerUnavailable, "OpenAPI Worker is not ready")
	}
	return nil
}

func (c *Client) EnsureSnapshotReady(ctx context.Context, snapshot SnapshotEnvelope) (DescribeResponse, error) {
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return DescribeResponse{}, NewError(ErrorCodeProtocolError, "cannot encode OpenAPI Worker snapshot")
	}
	request := DescribeRequest{
		ProtocolVersion: ProtocolVersion,
		RequestID:       NewRequestID(),
		ExecutionDigest: snapshot.ExecutionDigest,
		Limit:           64,
	}
	response, err := c.Describe(ctx, request)
	if err == nil {
		return c.describeRemainingPages(ctx, request, response)
	}
	workerErr, ok := AsError(err)
	if !ok || workerErr.Code != ErrorCodeSnapshotRequired {
		return DescribeResponse{}, err
	}
	request.Snapshot = encoded
	response, err = c.Describe(ctx, request)
	if err != nil {
		return DescribeResponse{}, err
	}
	return c.describeRemainingPages(ctx, request, response)
}

func (c *Client) describeRemainingPages(ctx context.Context, request DescribeRequest, response DescribeResponse) (DescribeResponse, error) {
	for !response.Complete {
		if response.NextCursor == nil || *response.NextCursor <= request.Cursor {
			return DescribeResponse{}, NewError(ErrorCodeProtocolError, "OpenAPI Worker returned invalid describe pagination")
		}
		request.RequestID = NewRequestID()
		request.Cursor = *response.NextCursor
		request.Snapshot = nil
		next, err := c.Describe(ctx, request)
		if err != nil {
			return DescribeResponse{}, err
		}
		response.Operations = append(response.Operations, next.Operations...)
		response.NextCursor = next.NextCursor
		response.Complete = next.Complete
	}
	return response, nil
}

func (c *Client) Describe(ctx context.Context, request DescribeRequest) (DescribeResponse, error) {
	var response DescribeResponse
	if request.ProtocolVersion != ProtocolVersion {
		return response, NewError(ErrorCodeProtocolError, "unsupported OpenAPI Worker protocol version")
	}
	if err := c.postJSON(ctx, describePath, request.RequestID, request, &response); err != nil {
		return DescribeResponse{}, err
	}
	if response.Kind != "describe" || response.Status != "ready" {
		return DescribeResponse{}, NewError(ErrorCodeProtocolError, "OpenAPI Worker describe response is not ready")
	}
	if response.ProtocolVersion != ProtocolVersion {
		return response, NewError(ErrorCodeProtocolError, "OpenAPI Worker protocol version mismatch")
	}
	if response.ExecutionDigest != request.ExecutionDigest {
		return DescribeResponse{}, NewError(ErrorCodeProtocolError, "OpenAPI Worker response execution digest mismatch")
	}
	return response, nil
}

func (c *Client) Execute(ctx context.Context, request ExecuteRequest) (ExecutionResult, error) {
	var response ExecutionResult
	if request.ProtocolVersion != ProtocolVersion {
		return response, NewError(ErrorCodeProtocolError, "unsupported OpenAPI Worker protocol version")
	}
	if request.DeadlineUnixMs <= time.Now().UnixMilli() {
		return response, NewError(ErrorCodeDeadlineExceeded, "execution deadline has already elapsed")
	}
	if err := c.postJSON(ctx, executePath, request.RequestID, request, &response); err != nil {
		return ExecutionResult{}, err
	}
	if response.RequestID != request.RequestID {
		return response, NewError(ErrorCodeProtocolError, "OpenAPI Worker response request ID mismatch")
	}
	if response.Kind != ResultKindBusiness || response.Outcome != "business_result" {
		return response, NewError(ErrorCodeProtocolError, "OpenAPI Worker returned an invalid business result")
	}
	return response, nil
}

func (c *Client) postJSON(ctx context.Context, path, requestID string, request, response any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return NewError(ErrorCodeProtocolError, "cannot encode OpenAPI Worker request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return NewError(ErrorCodeWorkerUnavailable, "invalid OpenAPI Worker request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Obot-Request-ID", requestID)
	if c.bearerToken != "" {
		req.Header.Set(workerAuthHeaderName, "Bearer "+c.bearerToken)
	}
	return c.do(req, response)
}

func (c *Client) getJSON(ctx context.Context, path string, response any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return NewError(ErrorCodeWorkerUnavailable, "invalid OpenAPI Worker health request")
	}
	req.Header.Set("Accept", "application/json")
	if c.bearerToken != "" {
		req.Header.Set(workerAuthHeaderName, "Bearer "+c.bearerToken)
	}
	return c.do(req, response)
}

func (c *Client) do(request *http.Request, response any) error {
	resp, err := c.httpClient.Do(request)
	if err != nil {
		return NewError(ErrorCodeWorkerUnavailable, "OpenAPI Worker request failed")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodyBytes+1))
	if err != nil {
		return NewError(ErrorCodeWorkerUnavailable, "cannot read OpenAPI Worker response")
	}
	if len(data) > MaxResponseBodyBytes {
		return NewError(ErrorCodeResponseTooLarge, "OpenAPI Worker response exceeds 8 MiB")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var workerError Error
		if json.Unmarshal(data, &workerError) == nil && workerError.Code != "" {
			workerError.RequestID = request.Header.Get("X-Obot-Request-ID")
			workerError.HTTPStatus = resp.StatusCode
			return &workerError
		}
		return &Error{
			Code:       ErrorCodeWorkerUnavailable,
			Message:    fmt.Sprintf("OpenAPI Worker returned HTTP %d", resp.StatusCode),
			HTTPStatus: http.StatusServiceUnavailable,
		}
	}
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		return NewError(ErrorCodeUpstreamProtocolUnsupported, "OpenAPI Worker returned a non-JSON response")
	}
	if err := json.Unmarshal(data, response); err != nil {
		return NewError(ErrorCodeProtocolError, "cannot decode OpenAPI Worker response")
	}
	return nil
}

func NewRequestID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	data[6] = (data[6] & 0x0f) | 0x40
	data[8] = (data[8] & 0x3f) | 0x80
	return hex.EncodeToString(data[:])
}

func (c *Client) Close() {
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

// DecodeBase64 is retained for callers that need to inspect bounded protocol bodies.
func DecodeBase64(value string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, NewError(ErrorCodeProtocolError, "invalid base64 protocol body")
	}
	if len(data) > MaxResponseBodyBytes {
		return nil, NewError(ErrorCodeResponseTooLarge, "protocol body exceeds 8 MiB")
	}
	return data, nil
}
