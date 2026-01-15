package mcp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"sync"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/gptscript-ai/gptscript/pkg/types"
	otypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Options struct {
	MCPBaseImage            string   `usage:"The base image to use for MCP containers" default:"ghcr.io/obot-platform/mcp-images/phat:main"`
	MCPHTTPWebhookBaseImage string   `usage:"The base image to use for HTTP-based MCP webhook containers" default:"ghcr.io/obot-platform/mcp-images/http-webhook-mcp-converter:main"`
	MCPRemoteShimBaseImage  string   `usage:"The base image to use for MCP remote shim containers" default:"ghcr.io/nanobot-ai/nanobot:v0.0.50"`
	MCPNamespace            string   `usage:"The namespace to use for MCP containers" default:"obot-mcp"`
	MCPClusterDomain        string   `usage:"The cluster domain to use for MCP containers" default:"cluster.local"`
	DisallowLocalhostMCP    bool     `usage:"Allow MCP containers to run on localhost"`
	MCPRuntimeBackend       string   `usage:"The runtime backend to use for running MCP servers: docker, kubernetes, or local. Defaults to docker." default:"docker"`
	MCPImagePullSecrets     []string `usage:"The name of the image pull secret to use for pulling MCP images"`

	// Kubernetes settings from Helm
	MCPK8sSettingsAffinity    string `usage:"Affinity rules for MCP server pods (JSON)" env:"OBOT_SERVER_MCPK8S_SETTINGS_AFFINITY"`
	MCPK8sSettingsTolerations string `usage:"Tolerations for MCP server pods (JSON)" env:"OBOT_SERVER_MCPK8S_SETTINGS_TOLERATIONS"`
	MCPK8sSettingsResources   string `usage:"Resource requests/limits for MCP server pods (JSON)" env:"OBOT_SERVER_MCPK8S_SETTINGS_RESOURCES"`

	// Obot service configuration for constructing internal service FQDN
	ServiceName      string `usage:"The Kubernetes service name for the obot server" env:"OBOT_SERVER_SERVICE_NAME"`
	ServiceNamespace string `usage:"The Kubernetes namespace where the obot server runs" env:"OBOT_SERVER_SERVICE_NAMESPACE"`

	// Audit log configuration
	MCPAuditLogPersistIntervalSeconds int `usage:"The interval in seconds to persist MCP audit logs to the database" default:"5"`
	MCPAuditLogsPersistBatchSize      int `usage:"The number of MCP audit logs to persist in a single batch" default:"1000"`
}

type SessionManager struct {
	backend           backend
	contextLock       sync.Mutex
	sessionCtx        context.Context
	cancel            func()
	sessions          sync.Map
	tokenService      TokenService
	baseURL           string
	allowLocalhostMCP bool

	webhookHelper *WebhookHelper
	gptClient     *gptscript.GPTScript
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

func NewSessionManager(ctx context.Context, tokenService TokenService, baseURL string, httpListenPort int, opts Options, localK8sConfig *rest.Config, obotStorageClient storage.Client) (*SessionManager, error) {
	var backend backend

	switch opts.MCPRuntimeBackend {
	case "docker":
		dockerBackend, err := newDockerBackend(ctx, httpListenPort, opts)
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

		backend = newKubernetesBackend(clientset, client, obotStorageClient, opts)
	default:
		return nil, fmt.Errorf("unknown runtime backend: %s", opts.MCPRuntimeBackend)
	}

	return &SessionManager{
		tokenService:      tokenService,
		backend:           backend,
		baseURL:           baseURL,
		allowLocalhostMCP: !opts.DisallowLocalhostMCP,
	}, nil
}

// Init must be called before the session manager is used.
func (sm *SessionManager) Init(gptClient *gptscript.GPTScript, webhookHelper *WebhookHelper) {
	sm.gptClient = gptClient
	sm.webhookHelper = webhookHelper
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
	if server.ProjectMCPServer {
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
	sm.contextLock.Lock()
	if sm.sessionCtx == nil {
		sm.contextLock.Unlock()
		return
	}
	sm.contextLock.Unlock()

	sessions, ok := sm.sessions.Load(server.MCPServerName)
	if !ok || sessions == nil {
		return
	}

	clientSessions, ok := sessions.(*sync.Map)
	if !ok || clientSessions == nil {
		return
	}

	sess, ok := clientSessions.LoadAndDelete(clientID(server) + clientScope)
	if !ok || sess == nil {
		return
	}

	if s, ok := sess.(*Client); ok && s.Client != nil {
		s.Close(false)
		s.Session.Wait()
	}
}

// LaunchServer will ensure that the server is deployed
func (sm *SessionManager) LaunchServer(ctx context.Context, serverConfig ServerConfig) (string, error) {
	if serverConfig.ProjectMCPServer {
		return "", errors.New("cannot launch project MCP server")
	}

	c, err := sm.ensureDeployment(ctx, serverConfig, true)
	return c.URL, err
}

// ShutdownServer will close the connections to the MCP server and remove the Kubernetes objects.
func (sm *SessionManager) ShutdownServer(ctx context.Context, serverName string) error {
	sm.closeClients(serverName)

	return sm.backend.shutdownServer(ctx, serverName)
}

func (sm *SessionManager) closeClients(serverName string) {
	sm.contextLock.Lock()
	if sm.sessionCtx == nil {
		sm.contextLock.Unlock()
		return
	}
	sm.contextLock.Unlock()

	sessions, ok := sm.sessions.LoadAndDelete(serverName)
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
	return sm.backend.restartServer(ctx, server.MCPServerName)
}

func (sm *SessionManager) ensureDeployment(ctx context.Context, server ServerConfig, transformRemote bool) (ServerConfig, error) {
	var webhooks []Webhook
	if !server.ComponentMCPServer {
		// Don't get webhooks for servers that are components of composite servers.
		// The webhooks would be called at the composite level.
		var err error
		webhooks, err = sm.webhookHelper.GetWebhooksForMCPServer(ctx, sm.gptClient, server)
		if err != nil {
			return ServerConfig{}, err
		}

		slices.SortFunc(webhooks, func(a, b Webhook) int {
			if a.Name < b.Name {
				return -1
			} else if a.Name > b.Name {
				return 1
			}
			return 0
		})
	}

	if server.Runtime == otypes.RuntimeRemote {
		if server.URL == "" {
			return ServerConfig{}, fmt.Errorf("MCP server %s needs to update its URL", server.MCPServerDisplayName)
		}

		if !sm.allowLocalhostMCP && !server.ProjectMCPServer && server.URL != "" {
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

		if !transformRemote || server.ProjectMCPServer {
			// If we aren't transforming the remote MCP server, then return it as is.
			return server, nil
		}
	}

	return sm.backend.ensureServerDeployment(ctx, server, webhooks)
}

func clientID(server ServerConfig) string {
	// The user ID and scope is not part of the client ID.
	server.UserID = ""
	return "mcp" + hash.Digest(server)
}

// GenerateToolPreviews creates a temporary MCP server from a catalog entry, lists its tools,
// then shuts it down and returns the tool preview data.
func (sm *SessionManager) GenerateToolPreviews(ctx context.Context, tempMCPServer v1.MCPServer, serverConfig ServerConfig) ([]otypes.MCPServerTool, error) {
	// Ensure cleanup happens regardless of success or failure
	defer func() {
		if cleanupErr := sm.ShutdownServer(ctx, serverConfig.MCPServerName); cleanupErr != nil {
			log.Errorf("failed to clean up temporary instance %s: %v", tempMCPServer.Name, cleanupErr)
		}
	}()

	// Use "system" for the user ID to identify non-user MCP servers.
	serverConfig.UserID = "system"

	// Create MCP client and list tools
	client, err := sm.clientForServer(ctx, serverConfig)
	if err != nil {
		return nil, err
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	return ConvertTools(tools.Tools, []string{"*"}, nil)
}

// GetCapacityInfo returns capacity information for the MCP namespace.
// Only available when using the Kubernetes backend.
func (sm *SessionManager) GetCapacityInfo(ctx context.Context) (otypes.MCPCapacityInfo, error) {
	if k8sBackend, ok := sm.backend.(*kubernetesBackend); ok {
		return k8sBackend.GetCapacityInfo(ctx), nil
	}
	return otypes.MCPCapacityInfo{}, &ErrNotSupportedByBackend{Feature: "capacity info", Backend: "docker"}
}
