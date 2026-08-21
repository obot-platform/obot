package mcp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/oasdiff/yaml"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/system"
)

const (
	defaultContainerPort          = 8099
	defaultWebhookToolName        = "fire-webhook"
	serviceUnavailableGracePeriod = 10 * time.Second

	runtimeBackendDocker          = "docker"
	RuntimeBackendKubernetes      = "kubernetes"
	runtimeBackendKubernetesShort = "k8s"
)

var (
	ErrHealthCheckTimeout     = errors.New("timed out waiting for MCP server to be ready")
	ErrHealthCheckFailed      = errors.New("MCP server is not healthy")
	ErrPodCrashLoopBackOff    = errors.New("pod is in CrashLoopBackOff state")
	ErrImagePullFailed        = errors.New("failed to pull container image")
	ErrPodSchedulingFailed    = errors.New("pod could not be scheduled")
	ErrPodConfigurationFailed = errors.New("pod configuration is invalid")
	ErrInsufficientCapacity   = errors.New("insufficient cluster capacity to deploy MCP server")
)

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

type mmmcpFileConfig struct {
	Servers []mmmcpFileServer `json:"servers" yaml:"servers"`
}

type mmmcpFileServer struct {
	Name               string            `json:"name" yaml:"name"`
	URL                string            `json:"url,omitempty" yaml:"url,omitempty"`
	Headers            map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	PassthroughHeaders []string          `json:"passthroughHeaders,omitempty" yaml:"passthroughHeaders,omitempty"`
	Command            string            `json:"command,omitempty" yaml:"command,omitempty"`
	Args               []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env                map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
}

func IsKubernetesBackend(backend string) bool {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case RuntimeBackendKubernetes, runtimeBackendKubernetesShort:
		return true
	default:
		return false
	}
}

func (e *ErrNotSupportedByBackend) Error() string {
	return fmt.Sprintf("feature %s is not supported by %s backend", e.Feature, e.Backend)
}

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
			if err := scanner.Err(); err != nil {
				lastErr = fmt.Errorf("failed reading SSE stream: %w", err)
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

// ServerHookConfig returns hook mappings and the MCP server configurations used
// by the native hook runner. The target MCP server itself is intentionally
// absent: its traffic continues through the reverse proxy.
func ServerHookConfig(server ServerConfig) (Hooks, HookServerConfigs) {
	replacer := strings.NewReplacer("/", "-", ":", "-", "?", "-")
	hooks := hookDefinitions(server.Webhooks, replacer)
	servers := make(HookServerConfigs, len(server.Webhooks))
	for _, webhook := range server.Webhooks {
		servers[webhookServerName(webhook, replacer)] = ServerConfig{
			Runtime:              types.RuntimeRemote,
			URL:                  webhook.URL,
			UserID:               server.UserID,
			MCPServerName:        system.SystemMCPServerPrefix + webhook.Name,
			MCPServerDisplayName: webhook.DisplayName,
			SystemMCPServer:      true,
			Audiences:            slices.Clone(server.Audiences),
		}
	}
	return hooks, servers
}

// MMMCPConfig converts a server configuration into mmmcp's runtime configuration.
func MMMCPConfig(server ServerConfig, env map[string][]byte) *mmmcpconfig.Config {
	if server.Runtime == types.RuntimeComposite {
		passthroughHeaders := make([]string, 0, len(server.PassthroughHeaderNames)+1)
		passthroughHeaders = append(passthroughHeaders, "Authorization")
		passthroughHeaders = append(passthroughHeaders, server.PassthroughHeaderNames...)

		servers := make([]mmmcpconfig.Server, 0, len(server.Components))
		for _, component := range server.Components {
			tools := make([]mmmcpconfig.ToolOverride, 0, len(component.Tools))
			for _, tool := range component.Tools {
				tools = append(tools, mmmcpconfig.ToolOverride{
					Name:                tool.Name,
					OverrideName:        tool.OverrideName,
					Description:         tool.Description,
					OverrideDescription: tool.OverrideDescription,
					Enabled:             tool.Enabled,
				})
			}

			servers = append(servers, mmmcpconfig.Server{
				Name:               component.DisplayName,
				Prefix:             component.ToolPrefix,
				URL:                component.URL,
				PassthroughHeaders: passthroughHeaders,
				Tools:              tools,
			})
		}

		return &mmmcpconfig.Config{Servers: servers}
	}

	replacer := strings.NewReplacer("/", "-", ":", "-", "?", "-")
	completeEnv := make(map[string]string, len(env))

	for k, v := range env {
		completeEnv[k] = string(v)
	}

	name := replacer.Replace(server.MCPServerDisplayName)
	if name == "" {
		name = replacer.Replace(server.MCPServerName)
	}

	return &mmmcpconfig.Config{Servers: []mmmcpconfig.Server{{
		Name:               name,
		URL:                server.URL,
		Headers:            keyValueSliceToMap(server.Headers),
		PassthroughHeaders: server.PassthroughHeaderNames,
		Command:            server.Command,
		Args:               slices.Clone(server.Args),
		Env:                completeEnv,
	}}}
}

func constructMCPServerMMMCPYAML(server ServerConfig, env map[string][]byte) ([]byte, error) {
	config := MMMCPConfig(server, env)
	fileConfig := mmmcpFileConfig{Servers: make([]mmmcpFileServer, 0, len(config.Servers))}
	for _, server := range config.Servers {
		args := make([]string, len(server.Args))
		for i, arg := range server.Args {
			args[i] = escapeMMMCPInterpolation(arg)
		}

		headers := make(map[string]string, len(server.Headers))
		for key, value := range server.Headers {
			headers[key] = escapeMMMCPInterpolation(value)
		}

		env := make(map[string]string, len(server.Env))
		for key, value := range server.Env {
			env[key] = escapeMMMCPInterpolation(value)
		}

		fileConfig.Servers = append(fileConfig.Servers, mmmcpFileServer{
			Name:               server.Name,
			URL:                escapeMMMCPInterpolation(server.URL),
			Headers:            headers,
			PassthroughHeaders: server.PassthroughHeaders,
			Command:            escapeMMMCPInterpolation(server.Command),
			Args:               args,
			Env:                env,
		})
	}

	data, err := yaml.Marshal(fileConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal mmmcp.yaml: %w", err)
	}

	return data, nil
}

// mmmcp treats ${NAME} as an environment reference and $$ as a literal dollar.
// Values in ServerConfig are already resolved, so preserve them literally.
func escapeMMMCPInterpolation(value string) string {
	return strings.ReplaceAll(value, "$", "$$")
}

func hookDefinitions(webhooks []Webhook, replacer *strings.Replacer) Hooks {
	definitions := make(Hooks, 0, len(webhooks))
	for _, webhook := range webhooks {
		webhookName := webhookServerName(webhook, replacer)
		targetName := webhookName + "/" + webhook.ToolName

		if len(webhook.Definitions) == 0 {
			definitions = append(definitions, HookMapping{
				Name:    "*",
				Targets: []HookTarget{{Target: targetName, MutateDisallowed: !webhook.MutateAllowed}},
			})
			continue
		}

		for _, def := range webhook.Definitions {
			method := def.Method
			if method == "" {
				method = "*"
			}
			if len(def.Identifiers) == 0 {
				definitions = append(definitions, HookMapping{
					Name:    method,
					Targets: []HookTarget{{Target: targetName, MutateDisallowed: !webhook.MutateAllowed}},
				})
			}
			for _, id := range def.Identifiers {
				var params map[string]string
				if id != "*" {
					params = map[string]string{"name": id}
				}
				definitions = append(definitions, HookMapping{
					Name:    method,
					Params:  params,
					Targets: []HookTarget{{Target: targetName, MutateDisallowed: !webhook.MutateAllowed}},
				})
			}
		}
	}

	return definitions
}

func webhookServerName(webhook Webhook, replacer *strings.Replacer) string {
	name := replacer.Replace(webhook.Name)
	if name == "" {
		name = replacer.Replace(webhook.DisplayName)
	}
	return name
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
