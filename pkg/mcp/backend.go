package mcp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nanobot-ai/nanobot/pkg/mcp"
	nanobottypes "github.com/nanobot-ai/nanobot/pkg/types"
	"github.com/oasdiff/yaml"
	"github.com/obot-platform/obot/apiclient/types"
)

const (
	defaultContainerPort = 8099
	webhookToolName      = "fire-webhook"
)

type backend interface {
	// ensureServerDeployment will deploy a server if it is not already deployed, and return the updated ServerConfig
	ensureServerDeployment(ctx context.Context, serverConfig ServerConfig, webhooks []Webhook) (ServerConfig, error)
	// deployServer will deploy a server if it is not already deployed, and will not wait or do any readiness checks
	deployServer(ctx context.Context, server ServerConfig, webhooks []Webhook) error
	transformConfig(ctx context.Context, serverConfig ServerConfig) (*ServerConfig, error)
	streamServerLogs(ctx context.Context, id string) (io.ReadCloser, error)
	getServerDetails(ctx context.Context, id string) (types.MCPServerDetails, error)
	restartServer(ctx context.Context, server ServerConfig) error
	shutdownServer(ctx context.Context, id string) error
	transformObotHostname(url string) string
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
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	client := &http.Client{
		Timeout: time.Second,
	}

	if server.Runtime != types.RuntimeContainerized || server.NanobotAgentName != "" {
		// This server is using nanobot as long as it is not the containerized runtime,
		// so we can reach out to nanobot's healthz path.
		url = fmt.Sprintf("%s/healthz", strings.TrimSuffix(url, "/"))
		for {
			resp, err := client.Get(url)
			if err == nil {
				resp.Body.Close()
				switch resp.StatusCode {
				case http.StatusOK:
					return nil
				case http.StatusServiceUnavailable:
					// The image will return a http.StatusTooEarly until it has finished trying to list tools.
					// If listing tools fails, it will return http.StatusServiceUnavailable.
					return ErrHealthCheckFailed
				}
			}

			select {
			case <-ctx.Done():
				return ErrHealthCheckTimeout
			case <-time.After(100 * time.Millisecond):
			}
		}
	}

	if server.ContainerPath != "" {
		// Try making a standard POST call to this MCP server to see if it responds.
		url = fmt.Sprintf("%s/%s", strings.TrimSuffix(url, "/"), strings.TrimPrefix(server.ContainerPath, "/"))
	}

	for {
		select {
		case <-ctx.Done():
			return ErrHealthCheckTimeout
		case <-time.After(100 * time.Millisecond):
		}

		req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(streamableHTTPHealthcheckBody))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Accept", "application/json,text/event-stream")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			continue
		}

		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			if sessionID := resp.Header.Get("Mcp-Session-Id"); sessionID != "" {
				// Send a cancellation, since we don't need this session.
				// If we get any errors, ignore them, because it doesn't matter for us.
				req, err := http.NewRequest(http.MethodDelete, url, nil)
				if err == nil {
					req.Header.Set("Mcp-Session-Id", sessionID)
					_, _ = http.DefaultClient.Do(req)
				}
			}
			return nil
		}

		// Fallback to trying SSE.
		req, err = http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Accept", "text/event-stream")

		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			continue
		}

		if resp.StatusCode == http.StatusOK {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

			// Start looking for an event with "endpoint".
			scanner := bufio.NewScanner(resp.Body)
		scannerLoop:
			for scanner.Scan() {
				select {
				case <-ctx.Done():
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

func webhookToServerConfig(webhook Webhook, baseImage, mcpServerName, userID, scope string, port int) (ServerConfig, error) {
	return ServerConfig{
		Runtime:              types.RuntimeContainerized,
		Scope:                scope,
		MCPServerName:        fmt.Sprintf("%s-%s", mcpServerName, webhook.Name),
		MCPServerDisplayName: webhook.DisplayName,
		UserID:               userID,
		ContainerImage:       baseImage,
		ContainerPort:        port,
		ContainerPath:        "/mcp",
		Env: []string{
			"WEBHOOK_URL=" + webhook.URL,
			"WEBHOOK_SECRET=" + webhook.Secret,
			"PORT=" + strconv.Itoa(port),
		},
	}, nil
}

func constructMCPServerNanobotYAMLForComposite(servers []ComponentServer) ([]byte, error) {
	mcpServers := make(map[string]mcp.Server, len(servers))
	names := make([]string, 0, len(servers))
	replacer := strings.NewReplacer("/", "-", ":", "-", "?", "-")

	for _, component := range servers {
		tools := make(map[string]mcp.ToolOverride, len(component.Tools))
		for _, tool := range component.Tools {
			if !tool.Enabled {
				continue
			}
			tools[tool.Name] = mcp.ToolOverride{
				Name:        tool.OverrideName,
				Description: tool.OverrideDescription,
			}
		}

		name := replacer.Replace(component.Name)
		mcpServers[name] = mcp.Server{
			BaseURL:       component.URL,
			ToolOverrides: tools,
		}

		names = append(names, name)
	}

	config := nanobottypes.Config{
		Publish: nanobottypes.Publish{
			MCPServers: names,
		},
		MCPServers: mcpServers,
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal nanobot.yaml: %w", err)
	}

	return data, nil
}

func constructMCPServerNanobotYAML(name, url, command string, args []string, env, headers map[string][]byte, webhooks []Webhook) ([]byte, error) {
	replacer := strings.NewReplacer("/", "-", ":", "-", "?", "-")

	hookTargets := make(map[string][]string, len(webhooks))
	mcpServers := make(map[string]mcp.Server, len(webhooks)+1)

	for _, webhook := range webhooks {
		webhookName := replacer.Replace(webhook.DisplayName)
		if webhookName == "" {
			webhookName = replacer.Replace(webhook.Name)
		}
		mcpServers[webhookName] = mcp.Server{
			BaseURL: webhook.URL,
		}
		for _, def := range webhook.Definitions {
			hookTargets[def] = append(hookTargets[def], fmt.Sprintf("%s/%s", webhookName, webhookToolName))
		}
	}

	hooks := make(mcp.Hooks, 0, len(hookTargets))
	for def, targets := range hookTargets {
		hooks = append(hooks, mcp.HookMapping{Name: def, Targets: targets})
	}

	name = replacer.Replace(name)
	mcpServers[name] = mcp.Server{
		BaseURL: url,
		Command: command,
		Args:    args,
		Env:     convertMapStringBytesToMapStringString(env),
		Headers: convertMapStringBytesToMapStringString(headers),
		Hooks:   hooks,
	}

	config := nanobottypes.Config{
		Publish: nanobottypes.Publish{
			MCPServers: []string{name},
		},
		MCPServers: mcpServers,
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal nanobot.yaml: %w", err)
	}

	return data, nil
}

func convertMapStringBytesToMapStringString(m map[string][]byte) map[string]string {
	if m == nil {
		return nil
	}

	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = string(v)
	}
	return result
}
