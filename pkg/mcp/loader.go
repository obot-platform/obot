package mcp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

type Options struct {
	MCPBaseImage                      string   `usage:"The base image to use for MCP containers" default:"ghcr.io/obot-platform/mcp-images/stdio-wrapper:v0.24.0"`
	MCPHTTPWebhookBaseImage           string   `usage:"The base image to use for HTTP-based MCP webhook containers" default:"ghcr.io/obot-platform/mcp-images/http-webhook-mcp-converter:v0.24.0"`
	MCPRemoteShimBaseImage            string   `usage:"The base image to use for MCP remote shim containers" default:"ghcr.io/obot-platform/nanobot:v0.0.87"`
	MCPNamespace                      string   `usage:"The namespace to use for MCP containers" default:"obot-mcp"`
	MCPClusterDomain                  string   `usage:"The cluster domain to use for MCP containers" default:"cluster.local"`
	DisallowLocalhostMCP              bool     `usage:"Disallow MCP containers from connecting to localhost" default:"true"`
	DisallowPrivateIPMCP              bool     `usage:"Disallow MCP containers from connecting to private IPs" default:"true"`
	DisallowLinkLocalMCP              bool     `usage:"Disallow MCP containers from connecting to link-local addresses" default:"true"`
	MCPRuntimeBackend                 string   `usage:"The runtime backend to use for running MCP servers: docker, kubernetes, or k8s. Defaults to docker" default:"docker"`
	MCPSecretBindingAllowedLabel      string   `usage:"Kubernetes Secret label key required for admin UI secret-binding lookup and save-time validation" default:"obot.obot.ai/allow-secret-binding"`
	MCPCACertFile                     string   `usage:"Path to a PEM CA certificate file to mount into MCP containers for custom TLS trust (Docker backend only)"`
	MCPCACertSecretName               string   `usage:"Name of a Kubernetes Secret holding a PEM CA bundle to mount into MCP pods for custom TLS trust (Kubernetes backend only)"`
	MCPCACertSecretKey                string   `usage:"Key within MCPCACertSecretName that holds the PEM CA bundle" default:"ca-bundle.crt"`
	MCPImagePullSecrets               []string `usage:"The name of the image pull secret to use for pulling MCP images"`
	SingleUserIdleServerShutdownHours int      `usage:"The interval in hours to check for idle MCP servers designated to a single user and shut them down, set to -1 to disable shutdown" default:"24"`
	MultiUserIdleServerShutdownHours  int      `usage:"The interval in hours to check for idle multi-user MCP servers and shut them down, set to -1 to disable" default:"168"`
	IdleAgentShutdownHours            int      `usage:"The interval in hours to check for idle agents and shut them down, set to -1 to disable" default:"72"`

	// Kubernetes settings from Helm
	MCPK8sSettingsAffinity              string `usage:"Affinity rules for MCP server pods (JSON)"`
	MCPK8sSettingsTolerations           string `usage:"Tolerations for MCP server pods (JSON)"`
	MCPK8sSettingsResources             string `usage:"Resource requests/limits for MCP server pods (JSON)"`
	MCPK8sSettingsNanobotAgentResources string `usage:"Resource requests/limits for NanobotAgent pods (JSON)"`
	MCPK8sSettingsRuntimeClassName      string `usage:"RuntimeClass name for MCP server pods (e.g., gvisor, kata)"`
	MCPK8sSettingsStorageClassName      string `usage:"StorageClass name for nanobot workspace volumes"`
	MCPK8sSettingsNanobotWorkspaceSize  string `usage:"Nanobot workspace size for MCP server pods (e.g., 1Gi)"`
	MCPK8sMaxCPURequest                 string `usage:"Maximum CPU request allowed for normal MCP server pods"`
	MCPK8sMaxCPULimit                   string `usage:"Maximum CPU limit allowed for normal MCP server pods"`
	MCPK8sMaxMemoryRequest              string `usage:"Maximum memory request allowed for normal MCP server pods"`
	MCPK8sMaxMemoryLimit                string `usage:"Maximum memory limit allowed for normal MCP server pods"`

	// Obot service configuration for constructing internal service FQDN
	ServiceName      string `usage:"The Kubernetes service name for the obot server"`
	ServiceNamespace string `usage:"The Kubernetes namespace where the obot server runs"`

	// Auto-populated by the Helm chart - used for network policy provider deployment
	ServiceAccountName string `usage:"The Kubernetes service account name for the obot server"`

	// Audit log configuration
	MCPAuditLogPersistIntervalSeconds int `usage:"The interval in seconds to persist MCP audit logs to the database" default:"5"`
	MCPAuditLogsPersistBatchSize      int `usage:"The number of MCP audit logs to persist in a single batch" default:"1000"`
	MCPAuditLogRetentionDays          int `usage:"The number of days to retain MCP audit logs (0 to disable cleanup)" default:"90"`

	// Pod Security Admission configuration for MCP namespace
	MCPPodSecurityEnabled        bool   `usage:"Enable Pod Security Admission labels on the MCP namespace" default:"true"`
	MCPPodSecurityEnforce        string `usage:"Pod Security Standards level to enforce (privileged, baseline, or restricted)" default:"restricted"`
	MCPPodSecurityEnforceVersion string `usage:"Kubernetes version for the enforce policy" default:"latest"`
	MCPPodSecurityAudit          string `usage:"Pod Security Standards level to audit (privileged, baseline, or restricted)" default:"restricted"`
	MCPPodSecurityAuditVersion   string `usage:"Kubernetes version for the audit policy" default:"latest"`
	MCPPodSecurityWarn           string `usage:"Pod Security Standards level to warn about (privileged, baseline, or restricted)" default:"restricted"`
	MCPPodSecurityWarnVersion    string `usage:"Kubernetes version for the warn policy" default:"latest"`
}

type SessionManager struct {
	backend                   backend
	runtimeBackend            string
	contextLock               sync.Mutex
	sessionCtx                context.Context
	cancel                    func()
	sessions                  sync.Map
	tokenService              TokenService
	baseURL                   string
	remoteURLValidationConfig RemoteMCPURLValidationConfig
	resourceMaximums          ResourceMaximums

	webhookHelper *WebhookHelper
}

type RemoteMCPURLValidationConfig struct {
	AllowLocalhostMCP bool
	AllowPrivateIPMCP bool
	AllowLinkLocalMCP bool
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

func NewSessionManager(ctx context.Context, authEnabled bool, tokenService TokenService, baseURL string, httpListenPort int, opts Options, webhookHelper *WebhookHelper, localK8sConfig *rest.Config, client, cachedClient, obotStorageClient kclient.WithWatch) (*SessionManager, error) {
	var backend backend
	resourceMaximums, err := ParseResourceMaximums(opts)
	if err != nil {
		return nil, err
	}

	switch opts.MCPRuntimeBackend {
	case runtimeBackendDocker:
		dockerBackend, err := newDockerBackend(ctx, authEnabled, httpListenPort, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize Docker backend: %w", err)
		}

		backend = dockerBackend
	case RuntimeBackendKubernetes, runtimeBackendKubernetesShort:
		if localK8sConfig == nil {
			return nil, fmt.Errorf("use of Kubernetes backend requested but no local K8s config available")
		}

		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: opts.MCPNamespace,
			},
		}

		// Add Pod Security Admission labels if enabled
		if opts.MCPPodSecurityEnabled {
			if namespace.Labels == nil {
				namespace.Labels = make(map[string]string)
			}
			namespace.Labels["pod-security.kubernetes.io/enforce"] = opts.MCPPodSecurityEnforce
			namespace.Labels["pod-security.kubernetes.io/enforce-version"] = opts.MCPPodSecurityEnforceVersion
			namespace.Labels["pod-security.kubernetes.io/audit"] = opts.MCPPodSecurityAudit
			namespace.Labels["pod-security.kubernetes.io/audit-version"] = opts.MCPPodSecurityAuditVersion
			namespace.Labels["pod-security.kubernetes.io/warn"] = opts.MCPPodSecurityWarn
			namespace.Labels["pod-security.kubernetes.io/warn-version"] = opts.MCPPodSecurityWarnVersion
		}

		if err := kclient.IgnoreAlreadyExists(client.Create(ctx, namespace)); err != nil {
			log.Warnf("failed to create MCP namespace, namespace must exist for MCP deployments to work: %v", err)
		}

		clientset, err := kubernetes.NewForConfig(localK8sConfig)
		if err != nil {
			return nil, err
		}

		backend = newKubernetesBackend(authEnabled, clientset, client, cachedClient, obotStorageClient, opts, resourceMaximums)
	default:
		return nil, fmt.Errorf("unknown runtime backend: %s", opts.MCPRuntimeBackend)
	}

	return &SessionManager{
		webhookHelper:    webhookHelper,
		tokenService:     tokenService,
		backend:          backend,
		runtimeBackend:   opts.MCPRuntimeBackend,
		baseURL:          baseURL,
		resourceMaximums: resourceMaximums,
		remoteURLValidationConfig: RemoteMCPURLValidationConfig{
			AllowLocalhostMCP: !opts.DisallowLocalhostMCP,
			AllowPrivateIPMCP: !opts.DisallowPrivateIPMCP,
			AllowLinkLocalMCP: !opts.DisallowLinkLocalMCP,
		},
	}, nil
}

func (sm *SessionManager) MCPRuntimeBackend() string {
	return sm.runtimeBackend
}

func (sm *SessionManager) RemoteMCPURLValidationConfig() RemoteMCPURLValidationConfig {
	return sm.remoteURLValidationConfig
}

func (sm *SessionManager) ResourceMaximums() ResourceMaximums {
	if sm == nil {
		return ResourceMaximums{}
	}
	return sm.resourceMaximums
}

func (sm *SessionManager) KubernetesResourceMaximums() ResourceMaximums {
	if sm == nil || !IsKubernetesBackend(sm.runtimeBackend) {
		return ResourceMaximums{}
	}
	return sm.resourceMaximums
}

func (sm *SessionManager) TransformObotHostname(hostname string) string {
	return sm.backend.transformObotHostname(hostname)
}

// Close does nothing with the deployments and services. It just closes the local session.
func (sm *SessionManager) Close() {
	sm.contextLock.Lock()
	if sm.sessionCtx == nil {
		sm.contextLock.Unlock()
		return
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
}

// CloseClient will close the client for this MCP server, but leave the deployment running.
func (sm *SessionManager) CloseClient(ctx context.Context, server ServerConfig, clientScope string) error {
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

	sess, ok := clientSessions.LoadAndDelete(clientID(server, clientScope))
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
	c, err := sm.ensureDeployment(ctx, serverConfig, true)
	return c.URL, err
}

// ShutdownServer will close the connections to the MCP server and remove all of the resources.
func (sm *SessionManager) ShutdownServer(ctx context.Context, serverName string) error {
	return sm.shutdownServer(ctx, serverName, true)
}

// ShutdownIdleServer will close the connections to the MCP server and remove all of the resources except for the volumes.
func (sm *SessionManager) ShutdownIdleServer(ctx context.Context, serverName string) error {
	return sm.shutdownServer(ctx, serverName, false)
}

func (sm *SessionManager) shutdownServer(ctx context.Context, serverName string, hardShutdown bool) error {
	sm.closeClients(serverName)

	return sm.backend.shutdownServer(ctx, serverName, hardShutdown)
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
	return sm.backend.restartServer(ctx, server)
}

func (sm *SessionManager) ensureDeployment(ctx context.Context, server ServerConfig, transformRemote bool) (ServerConfig, error) {
	var webhooks []Webhook
	if (server.Runtime != types.RuntimeRemote || transformRemote) && !server.ComponentMCPServer && !server.SystemMCPServer {
		// Don't get webhooks for servers that are components of composite servers.
		// The webhooks would be called at the composite level.
		var err error
		webhooks, err = sm.webhookHelper.GetWebhooksForMCPServer(server)
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

	if server.Runtime == types.RuntimeRemote {
		if server.URL == "" {
			return ServerConfig{}, fmt.Errorf("MCP server %s needs to update its URL", server.MCPServerDisplayName)
		}

		if err := ValidateRemoteMCPURL(ctx, server.URL, sm.RemoteMCPURLValidationConfig()); err != nil {
			return ServerConfig{}, err
		}

		if !transformRemote {
			// If we aren't transforming the remote MCP server, then return it as is.
			return server, nil
		}
	}

	ctx, cancel := context.WithTimeout(ctx, server.StartupTimeout)
	defer cancel()

	return sm.backend.ensureServerDeployment(ctx, server, webhooks)
}

// ValidateRemoteMCPURL rejects remote MCP URLs that resolve to blocked local address ranges.
func ValidateRemoteMCPURL(ctx context.Context, rawURL string, config RemoteMCPURLValidationConfig) error {
	if strings.TrimSpace(rawURL) == "" {
		return nil
	}
	if config.AllowLocalhostMCP && config.AllowPrivateIPMCP && config.AllowLinkLocalMCP {
		return nil
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("failed to parse MCP server URL: %w", err)
	}

	hostname := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if !config.AllowLocalhostMCP && (hostname == "localhost" || strings.HasSuffix(hostname, ".localhost")) {
		return fmt.Errorf("MCP server URL must not be a localhost URL: %s", rawURL)
	}

	// LookupHost handles literal IP addresses and hostnames consistently.
	addrs, err := net.DefaultResolver.LookupHost(ctx, hostname)
	if err != nil {
		return fmt.Errorf("failed to resolve MCP server URL hostname: %w", err)
	}

	for _, addr := range addrs {
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}

		if !config.AllowLocalhostMCP && ip.IsLoopback() {
			return fmt.Errorf("MCP server URL must not be a localhost URL: %s", rawURL)
		}
		if !config.AllowPrivateIPMCP && ip.IsPrivate() {
			return fmt.Errorf("MCP server URL must not resolve to a private IP address: %s", rawURL)
		}
		if !config.AllowLinkLocalMCP && (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) {
			return fmt.Errorf("MCP server URL must not resolve to a link-local address: %s", rawURL)
		}
	}

	return nil
}

func serverID(server ServerConfig) string {
	// The user ID is not part of the server ID.
	server.UserID = ""
	// Neither are the passthrough header values since they are per-user.
	server.PassthroughHeaderValues = nil

	// File values are dynamic and can be updated in place.
	// Keep file env keys, but clear file contents before hashing.
	files := make([]File, 0, len(server.Files))
	for _, f := range server.Files {
		if f.Dynamic {
			files = append(files, File{
				EnvKey: f.EnvKey,
			})
		} else {
			files = append(files, f)
		}
	}
	server.Files = files

	return "mcp" + utils.Digest(server)
}

func clientID(server ServerConfig, clientScope string) string {
	return serverID(server) + utils.Digest(server.PassthroughHeaderValues) + clientScope
}

// GenerateToolPreviews creates a temporary MCP server from a catalog entry, lists its tools,
// then shuts it down and returns the tool preview data.
func (sm *SessionManager) GenerateToolPreviews(ctx context.Context, tempMCPServer v1.MCPServer, serverConfig ServerConfig) ([]types.MCPServerTool, error) {
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

	return ConvertTools(tools.Tools, nil)
}

// GetCapacityInfo returns capacity information for the MCP namespace.
// Only available when using the Kubernetes backend.
func (sm *SessionManager) GetCapacityInfo(ctx context.Context) (types.MCPCapacityInfo, error) {
	if k8sBackend, ok := sm.backend.(*kubernetesBackend); ok {
		return k8sBackend.GetCapacityInfo(ctx), nil
	}
	return types.MCPCapacityInfo{}, &ErrNotSupportedByBackend{Feature: "capacity info", Backend: "docker"}
}
