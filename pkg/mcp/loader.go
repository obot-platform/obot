package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/nah/pkg/apply"
	otypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/wait"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var log = logger.Package()

type Options struct {
	MCPBaseImage         string `usage:"The base image to use for MCP containers"`
	MCPNamespace         string `usage:"The namespace to use for MCP containers" default:"obot-mcp"`
	MCPClusterDomain     string `usage:"The cluster domain to use for MCP containers" default:"cluster.local"`
	DisallowLocalhostMCP bool   `usage:"Allow MCP containers to run on localhost"`
}

type SessionManager struct {
	client                                             kclient.WithWatch
	clientset                                          kubernetes.Interface
	contextLock                                        sync.Mutex
	sessionCtx                                         context.Context
	cancel                                             func()
	sessions                                           sync.Map
	tokenStorage                                       GlobalTokenStore
	baseURL, baseImage, mcpNamespace, mcpClusterDomain string
	allowLocalhostMCP                                  bool
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

func NewSessionManager(ctx context.Context, tokenStorage GlobalTokenStore, baseURL string, opts Options) (*SessionManager, error) {
	var (
		client    kclient.WithWatch
		clientset kubernetes.Interface
	)
	if opts.MCPBaseImage != "" {
		config, err := buildConfig()
		if err != nil {
			return nil, err
		}

		client, err = kclient.NewWithWatch(config, kclient.Options{})
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

		clientset, err = kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}
	}

	return &SessionManager{
		client:            client,
		clientset:         clientset,
		tokenStorage:      tokenStorage,
		baseURL:           baseURL,
		baseImage:         opts.MCPBaseImage,
		mcpClusterDomain:  opts.MCPClusterDomain,
		mcpNamespace:      opts.MCPNamespace,
		allowLocalhostMCP: !opts.DisallowLocalhostMCP,
	}, nil
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
				s.Session.Close()
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
	if !sm.KubernetesEnabled() || server.Command == "" {
		sm.closeClient(server, clientScope)
		return nil
	}

	id := deploymentID(server)

	var pods corev1.PodList
	err := sm.client.List(ctx, &pods, &kclient.ListOptions{
		Namespace: sm.mcpNamespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app": id,
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to list MCP pods: %w", err)
	}

	if len(pods.Items) != 0 {
		// If the pod was removed, then this won't do anything. The session will only get cleaned up when the server restarts.
		// That's better than the alternative of having unusable sessions that users are still trying to use.
		sm.closeClient(ServerConfig{URL: fmt.Sprintf("http://%s.%s.svc.%s/%s", id, sm.mcpNamespace, sm.mcpClusterDomain, strings.TrimPrefix(server.ContainerPath, "/")), Scope: pods.Items[0].Name}, clientScope)
		return nil
	}

	return nil
}

func (sm *SessionManager) closeClient(server ServerConfig, clientScope string) {
	id := deploymentID(server)

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
		s.Session.Close()
		s.Session.Wait()
	}
}

// ShutdownServer will close the connections to the MCP server and remove the Kubernetes objects.
func (sm *SessionManager) ShutdownServer(ctx context.Context, server ServerConfig) error {
	id := deploymentID(server)
	sm.closeClients(id)

	if sm.client != nil {
		if err := apply.New(sm.client).WithNamespace(sm.mcpNamespace).WithOwnerSubContext(id).WithPruneTypes(new(corev1.Secret), new(appsv1.Deployment), new(corev1.Service)).Apply(ctx, nil, nil); err != nil {
			return fmt.Errorf("failed to delete MCP deployment %s: %w", id, err)
		}
	}
	return nil
}

func (sm *SessionManager) closeClients(id string) {
	sm.contextLock.Lock()
	if sm.sessionCtx == nil {
		sm.contextLock.Unlock()
		return
	}
	sm.contextLock.Unlock()

	sessions, ok := sm.sessions.LoadAndDelete(id)
	if !ok || sessions == nil {
		return
	}

	clientSessions, ok := sessions.(*sync.Map)
	if !ok || clientSessions == nil {
		return
	}

	clientSessions.Range(func(_, session any) bool {
		if s, ok := session.(*Client); ok && s.Client != nil {
			s.Session.Close()
			s.Session.Wait()
		}
		return true
	})
}

// RestartK8sDeployment restarts the Kubernetes deployment using kubectl rollout restart style.
// This patches the deployment with a restart annotation to trigger a rolling restart.
func (sm *SessionManager) RestartK8sDeployment(ctx context.Context, server ServerConfig) error {
	if server.Command == "" {
		return nil
	}
	id := deploymentID(server)

	var deployment appsv1.Deployment
	if err := sm.client.Get(ctx, kclient.ObjectKey{Name: id, Namespace: sm.mcpNamespace}, &deployment); err != nil {
		return fmt.Errorf("failed to get deployment %s: %w", id, err)
	}

	patch := map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]string{
						"kubectl.kubernetes.io/restartedAt": time.Now().Format(time.RFC3339),
					},
				},
			},
		},
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	if err := sm.client.Patch(ctx, &deployment, kclient.RawPatch(ktypes.MergePatchType, patchBytes)); err != nil {
		return fmt.Errorf("failed to patch deployment %s: %w", id, err)
	}

	return nil
}

func (sm *SessionManager) KubernetesEnabled() bool {
	return sm.client != nil
}

// Load is used by GPTScript to load tools from dynamic MCP server tool definitions.
// Obot is responsible for loading these tools and managing the clients and sessions.
// Error here to catch any server tools that slipped through. This should never be called.
func (sm *SessionManager) Load(_ context.Context, t types.Tool) ([]types.Tool, error) {
	return nil, fmt.Errorf("MCP servers must be loaded in Obot: %s", t.Name)
}

func (sm *SessionManager) ensureDeployment(ctx context.Context, server ServerConfig, serverName string) (ServerConfig, error) {
	if server.Runtime == otypes.RuntimeRemote && server.URL == "" {
		return ServerConfig{}, fmt.Errorf("MCP server %s needs to update its URL", serverName)
	}

	if server.Runtime == otypes.RuntimeRemote || !sm.KubernetesEnabled() {
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
		// Either we aren't deploying to Kubernetes, or this is a remote MCP server (so there is nothing to deploy to Kubernetes).
		return server, nil
	}

	// Generate the Kubernetes deployment objects.
	var (
		id   = deploymentID(server)
		objs []kclient.Object
		err  error
	)
	switch server.Runtime {
	case otypes.RuntimeNPX, otypes.RuntimeUVX:
		objs, err = sm.k8sObjectsForUVXOrNPX(server, serverName)
	case otypes.RuntimeContainerized:
		objs, err = sm.k8sObjectsForContainerized(server, serverName)
	default:
		return ServerConfig{}, fmt.Errorf("unsupported MCP runtime: %s", server.Runtime)
	}
	if err != nil {
		return ServerConfig{}, fmt.Errorf("failed to generate kubernetes objects for server %s: %w", id, err)
	}

	if err := apply.New(sm.client).WithNamespace(sm.mcpNamespace).WithOwnerSubContext(id).Apply(ctx, nil, objs...); err != nil {
		return ServerConfig{}, fmt.Errorf("failed to create MCP deployment %s: %w", id, err)
	}

	u := fmt.Sprintf("http://%s.%s.svc.%s", id, sm.mcpNamespace, sm.mcpClusterDomain)
	podName, err := sm.updatedMCPPodName(ctx, u, id, server)
	if err != nil {
		return ServerConfig{}, err
	}

	fullURL := fmt.Sprintf("%s/%s", u, strings.TrimPrefix(server.ContainerPath, "/"))

	// Use the pod name as the scope, so we get a new session if the pod restarts. MCP sessions aren't persistent on the server side.
	return ServerConfig{URL: fullURL, Scope: podName, AllowedTools: server.AllowedTools}, nil
}

func (sm *SessionManager) transformServerConfig(ctx context.Context, mcpServerName string, serverConfig ServerConfig) (ServerConfig, error) {
	return sm.ensureDeployment(ctx, serverConfig, mcpServerName)
}

func deploymentID(server ServerConfig) string {
	// The allowed tools and client scope aren't part of the deployment ID.
	server.AllowedTools = nil
	return "mcp" + hash.Digest(server)[:60]
}

func (sm *SessionManager) updatedMCPPodName(ctx context.Context, url, id string, server ServerConfig) (string, error) {
	// Wait for the deployment to be updated.
	_, err := wait.For(ctx, sm.client, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: id, Namespace: sm.mcpNamespace}}, func(dep *appsv1.Deployment) (bool, error) {
		return dep.Status.Replicas == 1 && dep.Status.UpdatedReplicas == 1 && dep.Status.ReadyReplicas == 1 && dep.Status.AvailableReplicas == 1, nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to wait for MCP server to be ready: %w", err)
	}

	// Ensure we can actually hit the service URL.
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	client := &http.Client{
		Timeout: time.Second,
	}

	if server.Runtime != otypes.RuntimeContainerized {
		// This server is using nanobot as long as it is not the containerized runtime,
		// so we can reach out to nanobot's healthz path.
		url = fmt.Sprintf("%s/healthz", url)
		for {
			resp, err := client.Get(url)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == 200 {
					break
				}
			}

			select {
			case <-ctx.Done():
				return "", fmt.Errorf("timed out waiting for MCP server to be ready")
			case <-time.After(100 * time.Millisecond):
			}
		}
	} else if server.ContainerPath != "" {
		// Try making a standard POST call to this MCP server to see if it responds.
		url = fmt.Sprintf("%s/%s", url, strings.TrimPrefix(server.ContainerPath, "/"))

	healthcheckLoop:
		for {
			select {
			case <-ctx.Done():
				return "", fmt.Errorf("timed out waiting for containerized MCP server to be ready")
			case <-time.After(100 * time.Millisecond):
			}

			req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(streamableHTTPHealthcheckBody))
			if err != nil {
				return "", fmt.Errorf("failed to create request: %w", err)
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
				break
			}

			// Fallback to trying SSE.
			req, err = http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				return "", fmt.Errorf("failed to create request: %w", err)
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
							break healthcheckLoop
						}
					}
				}
				resp.Body.Close()
				cancel()
			}
		}
	}

	// Not get the pod name that is currently running, waiting for there to only be one pod.
	var (
		pods            corev1.PodList
		runningPodCount int
		podName         string
	)
	for {
		if err = sm.client.List(ctx, &pods, &kclient.ListOptions{
			Namespace: sm.mcpNamespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"app": id,
			}),
		}); err != nil {
			return "", fmt.Errorf("failed to list MCP pods: %w", err)
		}

		runningPodCount = 0
		for _, p := range pods.Items {
			if p.Status.Phase == corev1.PodRunning {
				podName = p.Name
				runningPodCount++
			}
		}
		if runningPodCount == 1 {
			return podName, nil
		}

		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timed out waiting for MCP server to be ready")
		case <-time.After(time.Second):
		}
	}
}

func constructNanobotYAML(name, command string, args []string, env map[string]string) (string, error) {
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
	Publish    nanobotConfigPublish              `json:"publish,omitempty"`
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

func buildConfig() (*rest.Config, error) {
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}

	kubeconfig := filepath.Join(homedir.HomeDir(), ".kube", "config")
	if k := os.Getenv("KUBECONFIG"); k != "" {
		kubeconfig = k
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}
