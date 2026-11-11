package mcp

import (
	"context"
	"io"
	neturl "net/url"
	"os"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/openapiworker"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type fixedOpenAPIWorkerBackend struct {
	obotInternalBaseURL string
	validationOptions   ValidationOptions
	executor            openapiworker.Executor
	client              *openapiworker.Client
}

func newFixedOpenAPIWorkerBackend(_ context.Context, devMode bool, storageClient kclient.WithWatch, gatewayClient *client.Client) (*fixedOpenAPIWorkerBackend, error) {
	config := openapiworker.ClientConfig{
		BaseURL:         strings.TrimSpace(os.Getenv("MCPOpenAPIWorkerURL")),
		DevMode:         devMode,
		ServerCAFile:    strings.TrimSpace(os.Getenv("MCPOpenAPIWorkerServerCAFile")),
		ClientCertFile:  strings.TrimSpace(os.Getenv("MCPOpenAPIWorkerClientCertFile")),
		ClientKeyFile:   strings.TrimSpace(os.Getenv("MCPOpenAPIWorkerClientKeyFile")),
		BearerTokenFile: strings.TrimSpace(os.Getenv("MCPOpenAPIWorkerAuthTokenFile")),
		BearerToken:     strings.TrimSpace(os.Getenv("MCPOpenAPIWorkerAuthToken")),
	}
	workerClient, err := openapiworker.NewClient(config)
	if err != nil {
		return nil, err
	}
	resolver := openapiworker.NewResolver(storageClient, gatewayClient, gatewayClient)
	return &fixedOpenAPIWorkerBackend{
		obotInternalBaseURL: strings.TrimSpace(os.Getenv("MCPOpenAPIWorkerObotInternalURL")),
		validationOptions: ValidationOptions{
			DevMode:                      devMode,
			RemoteMCPURLValidationConfig: remoteConfigFromEnvironment(),
		},
		executor: openapiworker.NewWorkerExecutor(resolver, workerClient),
		client:   workerClient,
	}, nil
}

func remoteConfigFromEnvironment() RemoteMCPURLValidationConfig {
	return RemoteMCPURLValidationConfig{
		AllowLocalhostMCP: os.Getenv("MCPOpenAPIWorkerAllowLocalhost") == "true",
		AllowPrivateIPMCP: os.Getenv("MCPOpenAPIWorkerAllowPrivateIP") == "true",
		AllowLinkLocalMCP: os.Getenv("MCPOpenAPIWorkerAllowLinkLocal") == "true",
	}
}

func (b *fixedOpenAPIWorkerBackend) ensureServerDeployment(_ context.Context, serverConfig ServerConfig) (ServerConfig, error) {
	switch serverConfig.Runtime {
	case types.RuntimeOpenAPI, types.RuntimeRemote, types.RuntimeVMCP, types.RuntimeComposite:
		return serverConfig, nil
	default:
		return ServerConfig{}, b.notSupported("dynamic hosting")
	}
}

func (b *fixedOpenAPIWorkerBackend) deployServer(_ context.Context, server ServerConfig) error {
	switch server.Runtime {
	case types.RuntimeOpenAPI, types.RuntimeRemote, types.RuntimeVMCP, types.RuntimeComposite:
		return nil
	default:
		return b.notSupported("dynamic hosting")
	}
}

func (b *fixedOpenAPIWorkerBackend) streamServerLogs(context.Context, string) (io.ReadCloser, error) {
	return nil, b.notSupported("server logs")
}

func (b *fixedOpenAPIWorkerBackend) getServerDetails(context.Context, string) (types.MCPServerDetails, error) {
	return types.MCPServerDetails{}, b.notSupported("server details")
}

func (b *fixedOpenAPIWorkerBackend) restartServer(context.Context, ServerConfig) error {
	return b.notSupported("server restart")
}

func (b *fixedOpenAPIWorkerBackend) shutdownServer(context.Context, string, bool) error {
	return nil
}

func (b *fixedOpenAPIWorkerBackend) transformObotHostname(value string) string {
	if b.obotInternalBaseURL == "" || value == "" {
		return value
	}

	parsed, err := neturl.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return value
	}
	base, err := neturl.Parse(b.obotInternalBaseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return value
	}

	parsed.Scheme = base.Scheme
	parsed.Host = base.Host
	parsed.User = nil
	return parsed.String()
}

func (b *fixedOpenAPIWorkerBackend) remoteConfig(globalConfig RemoteMCPURLValidationConfig) (RemoteMCPURLValidationConfig, []string) {
	if b.obotInternalBaseURL == "" {
		return b.validationOptions.RemoteMCPURLValidationConfig, nil
	}
	base, err := neturl.Parse(b.obotInternalBaseURL)
	if err != nil || base.Host == "" {
		return b.validationOptions.RemoteMCPURLValidationConfig, nil
	}
	allowedHosts := []string{base.Host}
	for _, host := range strings.Split(os.Getenv("MCPOpenAPIWorkerAllowedHosts"), ",") {
		if host = strings.TrimSpace(host); host != "" {
			allowedHosts = append(allowedHosts, host)
		}
	}
	return b.validationOptions.RemoteMCPURLValidationConfig, allowedHosts
}

func (b *fixedOpenAPIWorkerBackend) notSupported(feature string) error {
	return &ErrNotSupportedByBackend{Feature: feature, Backend: RuntimeBackendOpenAPIWorker}
}

func (sm *SessionManager) OpenAPIWorkerExecutor() openapiworker.Executor {
	if sm == nil || sm.backend == nil {
		return nil
	}
	backend, ok := sm.backend.(*fixedOpenAPIWorkerBackend)
	if !ok {
		return nil
	}
	return backend.executor
}

func (sm *SessionManager) SetOpenAPIBudgetManager(manager openapiworker.BudgetManager) {
	if sm == nil || sm.backend == nil {
		return
	}
	backend, ok := sm.backend.(*fixedOpenAPIWorkerBackend)
	if !ok {
		return
	}
	if executor, ok := backend.executor.(*openapiworker.WorkerExecutor); ok {
		executor.ConfigureBudgetManager(manager)
	}
}

func (sm *SessionManager) CloseOpenAPIWorkerClient() {
	if sm == nil || sm.backend == nil {
		return
	}
	if backend, ok := sm.backend.(*fixedOpenAPIWorkerBackend); ok && backend.client != nil {
		backend.client.Close()
	}
}

var _ backend = (*fixedOpenAPIWorkerBackend)(nil)
