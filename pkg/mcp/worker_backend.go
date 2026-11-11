package mcp

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/openapiworker"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type fixedOpenAPIWorkerBackend struct {
	validationOptions ValidationOptions
	executor          openapiworker.Executor
	client            *openapiworker.Client
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
	return value
}

func (b *fixedOpenAPIWorkerBackend) remoteConfig(globalConfig RemoteMCPURLValidationConfig) (RemoteMCPURLValidationConfig, []string) {
	return globalConfig, nil
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

func (sm *SessionManager) CloseOpenAPIWorkerClient() {
	if sm == nil || sm.backend == nil {
		return
	}
	if backend, ok := sm.backend.(*fixedOpenAPIWorkerBackend); ok && backend.client != nil {
		backend.client.Close()
	}
}

var _ backend = (*fixedOpenAPIWorkerBackend)(nil)
