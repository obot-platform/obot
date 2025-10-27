package mcp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/gptscript-ai/gptscript/pkg/types"
	otypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/jwt/ephemeral"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var log = logger.Package()

type Options struct {
	MCPBaseImage         string   `usage:"The base image to use for MCP containers" default:"ghcr.io/obot-platform/mcp-images/phat:main"`
	MCPNamespace         string   `usage:"The namespace to use for MCP containers" default:"obot-mcp"`
	MCPClusterDomain     string   `usage:"The cluster domain to use for MCP containers" default:"cluster.local"`
	DisallowLocalhostMCP bool     `usage:"Allow MCP containers to run on localhost"`
	MCPRuntimeBackend    string   `usage:"The runtime backend to use for running MCP servers: docker, kubernetes, or local. Defaults to docker." default:"docker"`
	MCPImagePullSecrets  []string `usage:"The name of the image pull secret to use for pulling MCP images"`

	// Kubernetes settings from Helm
	MCPK8sSettingsAffinity    string `usage:"Affinity rules for MCP server pods (JSON)" env:"OBOT_SERVER_MCPK8S_SETTINGS_AFFINITY"`
	MCPK8sSettingsTolerations string `usage:"Tolerations for MCP server pods (JSON)" env:"OBOT_SERVER_MCPK8S_SETTINGS_TOLERATIONS"`
	MCPK8sSettingsResources   string `usage:"Resource requests/limits for MCP server pods (JSON)" env:"OBOT_SERVER_MCPK8S_SETTINGS_RESOURCES"`
}

type SessionManager struct {
	backend               backend
	contextLock           sync.Mutex
	sessionCtx            context.Context
	cancel                func()
	sessions              sync.Map
	tokenStorage          GlobalTokenStore
	ephemeralTokenService *ephemeral.TokenService
	baseURL               string
	allowLocalhostMCP     bool
}

const streamableHTTPHealthcheckBody string = `{
	"jsonrpc": "2.0",
	"id": "1",
    "method": "initialize",
    "params": {
        "capabilities": {},
        "clientInfo": {
            "name": "dummy",
            "version": "dummy"
        },
        "protocolVersion": "2025-06-18"
    }
}`

func NewSessionManager(ctx context.Context, ephemeralTokenService *ephemeral.TokenService, tokenStorage GlobalTokenStore, baseURL string, opts Options, localK8sConfig *rest.Config, obotStorageClient storage.Client) (*SessionManager, error) {
	var backend backend

	switch opts.MCPRuntimeBackend {
	case "docker":
		dockerBackend, err := newDockerBackend(ctx, opts.MCPBaseImage)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Docker backend: %w", err)
		}

		backend = dockerBackend
	case "kubernetes", "k8s":
		if localK8sConfig == nil {
			return nil, fmt.Errorf("use ofKubernetes backend requested but no local K8s config available")
		}

		client, err := kclient.NewWithWatch(localK8sConfig, kclient.Options{})
		if err != nil {
			return nil, err
		}

		if err = kclient.IgnoreAlreadyExists(client.Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: opts.MCPNamespace,
			},
		})); err != nil {
			log.Warnf("failed to create MCP namespace, namespace must exist for MCP deployments to work: %v", err)
		}

		clientset, err := kubernetes.NewForConfig(localK8sConfig)
		if err != nil {
			return nil, err
		}

		backend = newKubernetesBackend(clientset, client, opts.MCPBaseImage, opts.MCPNamespace, opts.MCPClusterDomain, opts.MCPImagePullSecrets, obotStorageClient)
	case "local":
		backend = newLocalBackend()
	default:
		return nil, fmt.Errorf("unknown runtime backend: %s", opts.MCPRuntimeBackend)
	}

	return &SessionManager{
		backend:               backend,
		tokenStorage:          tokenStorage,
		ephemeralTokenService: ephemeralTokenService,
		baseURL:               baseURL,
		allowLocalhostMCP:     !opts.DisallowLocalhostMCP,
	}, nil
}

// Load is used by GPTScript to load tools from dynamic MCP server tool definitions.
// Obot is responsible for loading these tools and managing the clients and sessions.
// Error here to catch any server tools that slipped through. This should never be called.
func (sm *SessionManager) Load(_ context.Context, t types.Tool) ([]types.Tool, error) {
	return nil, fmt.Errorf("MCP servers must be loaded in Obot: %s", t.Name)
}

// Close does nothing with the deployments and services. It just closes the local session.
// This should return an error to satisfy the GPTScript loader interface.
func (sm *SessionManager) Close() error {
	sm.contextLock.Lock()
	if sm.sessionCtx == nil {
		sm.contextLock.Unlock()
		return nil
	}
	sm.contextLock.Unlock()

	defer func() {
		sm.cancel()
		sm.contextLock.Lock()
		sm.sessionCtx = nil
		sm.contextLock.Unlock()
	}()

	sm.sessions.Range(func(id, value any) bool {
		value.(*sync.Map).Range(func(clientScope, session any) bool {
			if s, ok := session.(*Client); ok && s.Client != nil {
				log.Infof("closing MCP session %s, %s", id, clientScope)
				s.Session.Close(false)
				s.Session.Wait()
			}
			return true
		})
		return true
	})

	return nil
}

// CloseClient will close the client for this MCP server, but leave the deployment running.
func (sm *SessionManager) CloseClient(ctx context.Context, server ServerConfig, clientScope string) error {
	if server.Command == "" {
		sm.closeClient(server, clientScope)
		return nil
	}

	serverConfig, err := sm.backend.transformConfig(ctx, server)
	if err != nil {
		return fmt.Errorf("failed to transform MCP server config: %w", err)
	} else if serverConfig != nil {
		sm.closeClient(*serverConfig, clientScope)
	}
	return nil
}

func (sm *SessionManager) closeClient(server ServerConfig, clientScope string) {
	id := clientID(server)

	sm.contextLock.Lock()
	if sm.sessionCtx == nil {
		sm.contextLock.Unlock()
		return
	}
	sm.contextLock.Unlock()

	sessions, ok := sm.sessions.Load(id)
	if !ok || sessions == nil {
		return
	}

	clientSessions, ok := sessions.(*sync.Map)
	if !ok || clientSessions == nil {
		return
	}

	sess, ok := clientSessions.LoadAndDelete(clientScope)
	if !ok || sess == nil {
		return
	}

	if s, ok := sess.(*Client); ok && s.Client != nil {
		s.Close(false)
		s.Session.Wait()
	}
}

// ShutdownServer will close the connections to the MCP server and remove the Kubernetes objects.
func (sm *SessionManager) ShutdownServer(ctx context.Context, server ServerConfig) error {
	sm.closeClients(server)

	return sm.backend.shutdownServer(ctx, server.Scope)
}

func (sm *SessionManager) closeClients(server ServerConfig) {
	sm.contextLock.Lock()
	if sm.sessionCtx == nil {
		sm.contextLock.Unlock()
		return
	}
	sm.contextLock.Unlock()

	sessions, ok := sm.sessions.LoadAndDelete(clientID(server))
	if !ok || sessions == nil {
		return
	}

	clientSessions, ok := sessions.(*sync.Map)
	if !ok || clientSessions == nil {
		return
	}

	clientSessions.Range(func(_, session any) bool {
		if s, ok := session.(*Client); ok && s.Client != nil {
			s.Close(true)
			s.Session.Wait()
		}
		return true
	})
}

// RestartServerDeployment restarts the server in the currently used backend, if the backend supports it.
// If the backend does not support restarts, then an [ErrNotSupportedByBackend] error is returned.
func (sm *SessionManager) RestartServerDeployment(ctx context.Context, server ServerConfig) error {
	if server.Runtime == otypes.RuntimeRemote {
		return otypes.NewErrBadRequest("cannot restart deployment for remote MCP server")
	}
	return sm.backend.restartServer(ctx, server.Scope)
}

// CheckK8sSettingsStatus checks if a server's deployment uses current K8s settings.
// Returns true if the server needs to be redeployed with new K8s settings.
func (sm *SessionManager) CheckK8sSettingsStatus(ctx context.Context, serverConfig ServerConfig) (needsUpdate bool, deployedHash string, err error) {
	// Only Kubernetes backend supports this
	kb, ok := sm.backend.(*kubernetesBackend)
	if !ok {
		return false, "", &ErrNotSupportedByBackend{Feature: "K8s settings check", Backend: "non-kubernetes"}
	}

	// Get current K8s settings
	currentSettings, err := kb.getK8sSettings(ctx)
	if err != nil {
		return false, "", fmt.Errorf("failed to get current K8s settings: %w", err)
	}

	// Get deployment
	deployment, err := kb.getDeployment(ctx, serverConfig.Scope)
	if err != nil {
		return false, "", fmt.Errorf("failed to get deployment: %w", err)
	}

	// Compare hashes
	deployedHash = deployment.Annotations["obot.ai/k8s-settings-hash"]
	currentHash := computeK8sSettingsHash(currentSettings)

	return deployedHash != currentHash, deployedHash, nil
}

func (sm *SessionManager) ensureDeployment(ctx context.Context, server ServerConfig, userID, mcpServerDisplayName, mcpServerName string) (ServerConfig, error) {
	switch server.Runtime {
	case otypes.RuntimeRemote:
		if server.URL == "" {
			return ServerConfig{}, fmt.Errorf("MCP server %s needs to update its URL", mcpServerDisplayName)
		}

		if !sm.allowLocalhostMCP && server.URL != "" {
			// Ensure the URL is not a localhost URL.
			u, err := url.Parse(server.URL)
			if err != nil {
				return ServerConfig{}, fmt.Errorf("failed to parse MCP server URL: %w", err)
			}

			// LookupHost will properly detect IP addresses.
			addrs, err := net.DefaultResolver.LookupHost(ctx, u.Hostname())
			if err != nil {
				return ServerConfig{}, fmt.Errorf("failed to resolve MCP server URL hostname: %w", err)
			}

			for _, addr := range addrs {
				if ip := net.ParseIP(addr); ip != nil && ip.IsLoopback() {
					return ServerConfig{}, fmt.Errorf("MCP server URL must not be a localhost URL: %s", server.URL)
				}
			}
		}

		return server, nil
	case otypes.RuntimeComposite:
		server.URL = fmt.Sprintf("%s/mcp-connect/%s", sm.baseURL, mcpServerName)
		return server, nil
	}

	return sm.backend.ensureServerDeployment(ctx, server, userID, mcpServerDisplayName, mcpServerName)
}

func (sm *SessionManager) transformServerConfig(ctx context.Context, userID, mcpServerDisplayName, mcpServerName string, serverConfig ServerConfig) (ServerConfig, error) {
	return sm.ensureDeployment(ctx, serverConfig, userID, mcpServerDisplayName, mcpServerName)
}

func clientID(server ServerConfig) string {
	// The allowed tools aren't part of the client ID.
	server.AllowedTools = nil
	return "mcp" + hash.Digest(server)
}

// GenerateToolPreviews creates a temporary MCP server from a catalog entry, lists its tools,
// then shuts it down and returns the tool preview data.
func (sm *SessionManager) GenerateToolPreviews(ctx context.Context, tempMCPServer v1.MCPServer, serverConfig ServerConfig) ([]otypes.MCPServerTool, error) {
	// Ensure cleanup happens regardless of success or failure
	defer func() {
		if cleanupErr := sm.ShutdownServer(ctx, serverConfig); cleanupErr != nil {
			log.Errorf("failed to clean up temporary instance %s: %v", tempMCPServer.Name, cleanupErr)
		}
	}()

	// Create MCP client and list tools
	client, err := sm.ClientForServer(ctx, "system", tempMCPServer.Spec.Manifest.Name, tempMCPServer.Name, serverConfig)
	if err != nil {
		return nil, err
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	return ConvertTools(tools.Tools, []string{"*"}, nil)
}

func constructNanobotYAML(name, command string, args []string, env map[string]string) (string, error) {
	name = strings.ReplaceAll(name, "/", "-")
	config := nanobotConfig{
		Publish: nanobotConfigPublish{
			MCPServers: []string{name},
		},
		MCPServers: map[string]nanobotConfigMCPServer{
			name: {
				Command: command,
				Args:    args,
				Env:     env,
			},
		},
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("failed to marshal nanobot.yaml: %w", err)
	}

	return string(data), nil
}

type nanobotConfig struct {
	Publish    nanobotConfigPublish              `json:"publish,omitzero"`
	MCPServers map[string]nanobotConfigMCPServer `json:"mcpServers,omitempty"`
}

type nanobotConfigPublish struct {
	MCPServers []string `json:"mcpServers,omitempty"`
}

type nanobotConfigMCPServer struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}
