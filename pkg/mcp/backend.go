package mcp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/oasdiff/yaml"
	"github.com/obot-platform/obot/apiclient/types"
)

const (
	defaultContainerPort          = 8099
	defaultWebhookToolName        = "fire-webhook"
	serviceUnavailableGracePeriod = 10 * time.Second
)

func IsKubernetesBackend(backend string) bool {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "kubernetes", "k8s":
		return true
	default:
		return false
	}
}

type backend interface {
	// ensureServerDeployment will deploy a server if it is not already deployed, and return the updated ServerConfig
	ensureServerDeployment(ctx context.Context, serverConfig ServerConfig, webhooks []Webhook) (ServerConfig, error)
	// deployServer will deploy a server if it is not already deployed, and will not wait or do any readiness checks
	deployServer(ctx context.Context, server ServerConfig, webhooks []Webhook) error
	transformConfig(ctx context.Context, serverConfig ServerConfig) (*ServerConfig, error)
	streamServerLogs(ctx context.Context, id string) (io.ReadCloser, error)
	getServerDetails(ctx context.Context, id string) (types.MCPServerDetails, error)
	restartServer(ctx context.Context, server ServerConfig) error
	shutdownServer(ctx context.Context, id string, hardShutdown bool) error
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
	client := &http.Client{
		Timeout: time.Second,
	}

	if server.Runtime != types.RuntimeContainerized || server.NanobotAgentName != "" {
		// This server is using nanobot as long as it is not the containerized runtime,
		// so we can reach out to nanobot's healthz path.
		url = fmt.Sprintf("%s/healthz", strings.TrimSuffix(url, "/"))
		var firstServiceUnavailable time.Time

		for {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
			if err != nil {
				return fmt.Errorf("failed to create request: %w", err)
			}

			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				switch resp.StatusCode {
				case http.StatusOK:
					return nil
				case http.StatusServiceUnavailable:
					// Older nanobot versions return 503 when tool listing permanently fails, but service mesh sidecars
					// (e.g. Istio's envoy) also return 503 during startup. To avoid confusing the two, we don't treat 503
					// as a permanent failure until we've seen consecutive 503 responses for this duration.
					// Current nanobot returns 500 instead, which is handled as an immediate failure below.
					if firstServiceUnavailable.IsZero() {
						firstServiceUnavailable = time.Now()
					} else if time.Since(firstServiceUnavailable) > serviceUnavailableGracePeriod {
						return ErrHealthCheckFailed
					}
				case http.StatusInternalServerError:
					// Nanobot returns 500 when tool listing permanently fails.
					return ErrHealthCheckFailed
				default:
					// A non-503 response (e.g. 425 TooEarly) means we're reaching the actual
					// nanobot process, not a proxy. Reset the grace period so that any subsequent
					// 503 gets a fresh window.
					firstServiceUnavailable = time.Time{}
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

		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
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

func constructMCPServerNanobotYAMLForComposite(servers []ComponentServer) ([]byte, error) {
	mcpServers := make(map[string]nanobotConfigMCPServer, len(servers))
	names := make([]string, 0, len(servers))
	replacer := strings.NewReplacer("/", "-", ":", "-", "?", "-")

	for _, component := range servers {
		tools := make(map[string]toolOverride, len(component.Tools))
		for _, tool := range component.Tools {
			if !tool.Enabled {
				continue
			}
			tools[tool.Name] = toolOverride{
				Name:        tool.OverrideName,
				Description: tool.OverrideDescription,
			}
		}

		name := replacer.Replace(component.Name)
		mcpServers[name] = nanobotConfigMCPServer{
			BaseURL:       component.URL,
			ToolOverrides: tools,
			ToolPrefix:    component.ToolPrefix,
		}

		names = append(names, name)
	}

	config := nanobotConfig{
		Publish: nanobotConfigPublish{
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

func constructMCPServerNanobotYAML(name, url, command string, args, passthroughHeaders []string, env, headers map[string][]byte, webhooks []Webhook) ([]byte, error) {
	replacer := strings.NewReplacer("/", "-", ":", "-", "?", "-")

	webhookDefinitions := make(map[string][]string, len(webhooks))
	mcpServers := make(map[string]nanobotConfigMCPServer, len(webhooks)+1)

	for _, webhook := range webhooks {
		webhookName := replacer.Replace(webhook.DisplayName)
		if webhookName == "" {
			webhookName = replacer.Replace(webhook.Name)
		}
		mcpServers[webhookName] = nanobotConfigMCPServer{
			BaseURL: webhook.URL,
		}

		if !webhook.MutateAllowed {
			webhookName = "!mutate:" + webhookName
		}
		for _, def := range webhook.Definitions {
			webhookDefinitions[def] = append(webhookDefinitions[def], fmt.Sprintf("%s/%s", webhookName, webhook.ToolName))
		}
	}

	name = replacer.Replace(name)
	mcpServers[name] = nanobotConfigMCPServer{
		BaseURL:            url,
		Command:            command,
		Args:               args,
		Env:                convertMapStringBytesToMapStringString(env),
		Headers:            convertMapStringBytesToMapStringString(headers),
		PassthroughHeaders: passthroughHeaders,
		Hooks:              webhookDefinitions,
	}

	config := nanobotConfig{
		Publish: nanobotConfigPublish{
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

type nanobotConfig struct {
	Publish    nanobotConfigPublish              `json:"publish,omitzero"`
	MCPServers map[string]nanobotConfigMCPServer `json:"mcpServers,omitempty"`
}

type nanobotConfigPublish struct {
	MCPServers []string `json:"mcpServers,omitempty"`
}

type nanobotConfigMCPServer struct {
	Command            string              `json:"command,omitempty"`
	Args               []string            `json:"args,omitempty"`
	Hooks              map[string][]string `json:"hooks,omitempty"`
	Env                map[string]string   `json:"env,omitempty"`
	Headers            map[string]string   `json:"headers,omitempty"`
	PassthroughHeaders []string            `json:"passthroughHeaders,omitempty"`
	BaseURL            string              `json:"url,omitempty"`

	ToolOverrides map[string]toolOverride `json:"toolOverrides,omitempty"`
	ToolPrefix    string                  `json:"toolPrefix,omitempty"`
}

type toolOverride struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}
