package mcp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oasdiff/yaml"
	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	ntypes "github.com/obot-platform/nanobot/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
)

const (
	defaultContainerPort          = 8099
	defaultWebhookToolName        = "fire-webhook"
	serviceUnavailableGracePeriod = 10 * time.Second

	runtimeBackendDocker          = "docker"
	RuntimeBackendKubernetes      = "kubernetes"
	runtimeBackendKubernetesShort = "k8s"
)

func IsKubernetesBackend(backend string) bool {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case RuntimeBackendKubernetes, runtimeBackendKubernetesShort:
		return true
	default:
		return false
	}
}

type backend interface {
	// ensureServerDeployment will deploy a server if it is not already deployed, and return the updated ServerConfig
	ensureServerDeployment(ctx context.Context, serverConfig ServerConfig) (ServerConfig, error)
	// deployServer will deploy a server if it is not already deployed, and will not wait or do any readiness checks
	deployServer(ctx context.Context, server ServerConfig) error
	streamServerLogs(ctx context.Context, id string) (io.ReadCloser, error)
	getServerDetails(ctx context.Context, id string) (types.MCPServerDetails, error)
	restartServer(ctx context.Context, server ServerConfig) error
	shutdownServer(ctx context.Context, id string, hardShutdown bool) error
	transformObotHostname(url string) string
	remoteConfig(globalConfig RemoteMCPURLValidationConfig) (RemoteMCPURLValidationConfig, []string)
}

type ErrNotSupportedByBackend struct {
	Feature, Backend string
}

func (e *ErrNotSupportedByBackend) Error() string {
	return fmt.Sprintf("feature %s is not supported by %s backend", e.Feature, e.Backend)
}

var (
	ErrHealthCheckTimeout     = errors.New("timed out waiting for MCP server to be ready")
	ErrHealthCheckFailed      = errors.New("MCP server is not healthy")
	ErrPodCrashLoopBackOff    = errors.New("pod is in CrashLoopBackOff state")
	ErrImagePullFailed        = errors.New("failed to pull container image")
	ErrPodSchedulingFailed    = errors.New("pod could not be scheduled")
	ErrPodConfigurationFailed = errors.New("pod configuration is invalid")
	ErrInsufficientCapacity   = errors.New("insufficient cluster capacity to deploy MCP server")
)

func ensureServerReady(ctx context.Context, url string, server ServerConfig) error {
	// Ensure we can actually hit the service URL.
	client := &http.Client{
		Timeout: time.Second,
	}

	if server.HealthzPath != "" {
		return ensureHTTPGetOK(ctx, client, urlWithPath(url, server.HealthzPath))
	}

	if server.ContainerPath != "" {
		// Try making a standard POST call to this MCP server to see if it responds.
		url = fmt.Sprintf("%s/%s", strings.TrimSuffix(url, "/"), strings.TrimPrefix(server.ContainerPath, "/"))
	}

	// This must be a non-nil error because Go does weird things when you use %w with a nil error.
	lastErr := errors.New("MCP server did not respond to health check")
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: last error was %w", ErrHealthCheckTimeout, lastErr)
		case <-time.After(100 * time.Millisecond):
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(streamableHTTPHealthcheckBody))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Accept", "application/json,text/event-stream")
		req.Header.Set("Content-Type", "application/json")
		copyHeaders(req.Header, server.PassthroughHeaderNames, server.PassthroughHeaderValues)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if sessionID := resp.Header.Get("Mcp-Session-Id"); sessionID != "" {
				// Send a cancellation, since we don't need this session.
				// If we get any errors, ignore them, because it doesn't matter for us.
				req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
				if err == nil {
					req.Header.Set("Mcp-Session-Id", sessionID)
					copyHeaders(req.Header, server.PassthroughHeaderNames, server.PassthroughHeaderValues)
					_, _ = http.DefaultClient.Do(req)
				}
			}
			return nil
		}

		// We know here that we have a non-200 response.
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		lastErr = fmt.Errorf("unexpected status code [%d]: %s", resp.StatusCode, string(body))

		// Fallback to trying SSE.
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Accept", "text/event-stream")
		copyHeaders(req.Header, server.PassthroughHeaderNames, server.PassthroughHeaderValues)

		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)

			// Start looking for an event with "endpoint".
			scanner := bufio.NewScanner(resp.Body)
		scannerLoop:
			for scanner.Scan() {
				select {
				case <-readCtx.Done():
					break scannerLoop
				default:
					if strings.Contains(scanner.Text(), "endpoint") {
						resp.Body.Close()
						cancel()
						return nil
					}
				}
			}
			resp.Body.Close()
			cancel()
		}
	}
}

func ensureHTTPGetOK(ctx context.Context, client *http.Client, url string) error {
	var firstServiceUnavailable time.Time
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		var (
			body []byte
			// This must be a non-nil error because Go does weird things when you use %w with a nil error.
			lastErr = errors.New("MCP server did not respond to health check")
		)
		resp, err := client.Do(req)
		if err == nil {
			body, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			switch resp.StatusCode {
			case http.StatusOK:
				return nil
			case http.StatusServiceUnavailable:
				lastErr = fmt.Errorf("service unavailable: %s", string(body))
				// Older nanobot versions return 503 when tool listing permanently fails, but service mesh sidecars
				// (e.g. Istio's envoy) also return 503 during startup. To avoid confusing the two, we don't treat 503
				// as a permanent failure until we've seen consecutive 503 responses for this duration.
				// Current nanobot returns 500 instead, which is handled as an immediate failure below.
				if firstServiceUnavailable.IsZero() {
					firstServiceUnavailable = time.Now()
				} else if time.Since(firstServiceUnavailable) > serviceUnavailableGracePeriod {
					return fmt.Errorf("%w: %v", ErrHealthCheckFailed, lastErr)
				}

			case http.StatusInternalServerError:
				lastErr = fmt.Errorf("internal server error: %s", string(body))
				// Nanobot returns 500 when tool listing permanently fails.
				return fmt.Errorf("%w: %v", ErrHealthCheckFailed, lastErr)
			default:
				lastErr = fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
				// A non-503 response (e.g. 425 TooEarly) means we're reaching the actual
				// nanobot process, not a proxy. Reset the grace period so that any subsequent
				// 503 gets a fresh window.
				firstServiceUnavailable = time.Time{}
			}
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("%w: %v", ErrHealthCheckFailed, lastErr)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func urlWithPath(urlStr, path string) string {
	u, err := url.Parse(urlStr)
	if err != nil {
		return urlStr
	}
	u.Path = path
	return u.String()
}

func compositeMCPServerNanobotConfig(server ServerConfig) ntypes.Config {
	names := make([]string, 0, len(server.Components))
	mcpServers := make(map[string]nmcp.Server, len(server.Components))

	replacer := strings.NewReplacer("/", "-", ":", "-", "?", "-")

	for _, component := range server.Components {
		var tools map[string]nmcp.ToolOverride
		for _, tool := range component.Tools {
			if !tool.Enabled {
				continue
			}
			if tools == nil {
				tools = make(map[string]nmcp.ToolOverride, len(component.Tools))
			}
			tools[tool.Name] = nmcp.ToolOverride{
				Name:        tool.OverrideName,
				Description: tool.OverrideDescription,
			}
		}

		name := replacer.Replace(component.Name)
		mcpServers[name] = nmcp.Server{
			BaseURL:       component.URL,
			ToolOverrides: tools,
			NoTools:       component.noTools,
			ToolPrefix:    component.ToolPrefix,
		}

		names = append(names, name)
	}

	return ntypes.Config{
		Publish: ntypes.Publish{
			MCPServers: names,
		},
		MCPServers: mcpServers,
	}
}

func constructMCPServerNanobotYAMLForComposite(server ServerConfig) ([]byte, error) {
	data, err := yaml.Marshal(compositeMCPServerNanobotConfig(server))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal nanobot.yaml: %w", err)
	}

	return data, nil
}

func ServerNanobotConfig(server ServerConfig, isComposite bool) ntypes.Config {
	var config ntypes.Config
	if isComposite {
		config = compositeMCPServerNanobotConfig(server)
	} else {
		config = serverNanobotConfig(server, nil)
	}

	config.Auth = &ntypes.Auth{
		OAuthClientID:     server.TokenExchangeClientID,
		OAuthClientSecret: server.TokenExchangeClientSecret,
	}

	return config
}

func serverNanobotConfig(server ServerConfig, env map[string][]byte) ntypes.Config {
	replacer := strings.NewReplacer("/", "-", ":", "-", "?", "-")

	webhookDefinitions, mcpServers := webhookDefinitions(server.Webhooks, replacer)

	completeEnv := maps.Clone(keyValueSliceToMap(server.Env))
	if completeEnv == nil {
		completeEnv = make(map[string]string, len(env))
	}

	for k, v := range env {
		completeEnv[k] = string(v)
	}

	name := replacer.Replace(server.MCPServerDisplayName)
	mcpServers[name] = nmcp.Server{
		BaseURL:            server.URL,
		Command:            server.Command,
		Args:               server.Args,
		Env:                completeEnv,
		Headers:            keyValueSliceToMap(server.Headers),
		PassthroughHeaders: server.PassthroughHeaderNames,
		Hooks:              webhookDefinitions,
	}

	return ntypes.Config{
		Publish: ntypes.Publish{
			MCPServers: []string{name},
		},
		MCPServers: mcpServers,
	}
}

func constructMCPServerNanobotYAML(server ServerConfig, env map[string][]byte) ([]byte, error) {
	// Don't include webhooks in the nanobot.yaml file in the MCP server. They belong in the proxy.
	server.Webhooks = nil
	data, err := yaml.Marshal(serverNanobotConfig(server, env))
	if err != nil {
		return nil, fmt.Errorf("failed to marshal nanobot.yaml: %w", err)
	}

	return data, nil
}

func webhookDefinitions(webhooks []Webhook, replacer *strings.Replacer) (nmcp.Hooks, map[string]nmcp.Server) {
	webhookDefinitions := make(nmcp.Hooks, 0, len(webhooks))
	mcpServers := make(map[string]nmcp.Server, len(webhooks)+1)

	for _, webhook := range webhooks {
		webhookName := replacer.Replace(webhook.DisplayName)
		if webhookName == "" {
			webhookName = replacer.Replace(webhook.Name)
		}
		mcpServers[webhookName] = nmcp.Server{
			BaseURL: webhook.URL,
		}

		targetName := webhookName + "/" + webhook.ToolName

		if len(webhook.Definitions) == 0 {
			webhookDefinitions = append(webhookDefinitions, nmcp.HookMapping{
				Name:    "*",
				Targets: []nmcp.HookTarget{{Target: targetName, MutateDisallowed: !webhook.MutateAllowed}},
			})
			continue
		}

		for _, def := range webhook.Definitions {
			if len(def.Identifiers) == 0 {
				webhookDefinitions = append(webhookDefinitions, nmcp.HookMapping{
					Name:    def.Method,
					Targets: []nmcp.HookTarget{{Target: targetName, MutateDisallowed: !webhook.MutateAllowed}},
				})
			}
			for _, id := range def.Identifiers {
				webhookDefinitions = append(webhookDefinitions, nmcp.HookMapping{
					Name:    def.Method,
					Params:  map[string]string{"name": id},
					Targets: []nmcp.HookTarget{{Target: targetName, MutateDisallowed: !webhook.MutateAllowed}},
				})
			}
		}
	}

	return webhookDefinitions, mcpServers
}

func keyValueSliceToMap(values []string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	result := make(map[string]string, len(values))
	for _, value := range values {
		if k, v, ok := strings.Cut(value, "="); ok && v != "" {
			result[k] = v
		}
	}
	return result
}
