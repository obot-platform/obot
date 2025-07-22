package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	gmcp "github.com/gptscript-ai/gptscript/pkg/mcp"
	"github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/wait"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var log = logger.Package()

type Options struct {
	MCPBaseImage               string   `usage:"The base image to use for MCP containers"`
	MCPNamespace               string   `usage:"The namespace to use for MCP containers" default:"obot-mcp"`
	MCPClusterDomain           string   `usage:"The cluster domain to use for MCP containers" default:"cluster.local"`
	AllowedMCPDockerImageRepos []string `usage:"The docker image repos to allow for MCP containers" split:"true"`
	DisallowLocalhostMCP       bool     `usage:"Allow MCP containers to run on localhost"`
}

type SessionManager struct {
	client                                    kclient.WithWatch
	clientset                                 kubernetes.Interface
	tokenStorage                              GlobalTokenStore
	local                                     *gmcp.Local
	baseImage, mcpNamespace, mcpClusterDomain string
	allowedDockerImageRepos                   []string
	allowLocalhostMCP                         bool
}

func NewSessionManager(ctx context.Context, defaultLoader *gmcp.Local, tokenStorage GlobalTokenStore, opts Options) (*SessionManager, error) {
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
		client:                  client,
		clientset:               clientset,
		local:                   defaultLoader,
		tokenStorage:            tokenStorage,
		baseImage:               opts.MCPBaseImage,
		mcpClusterDomain:        opts.MCPClusterDomain,
		mcpNamespace:            opts.MCPNamespace,
		allowedDockerImageRepos: opts.AllowedMCPDockerImageRepos,
		allowLocalhostMCP:       !opts.DisallowLocalhostMCP,
	}, nil
}

// Close does nothing with the deployments and services. It just closes the local session.
func (sm *SessionManager) Close() error {
	return sm.local.Close()
}

// CloseClient will close the client for this MCP server, but leave the server running.
func (sm *SessionManager) CloseClient(ctx context.Context, server ServerConfig) error {
	if !sm.KubernetesEnabled() || server.Command == "" {
		return sm.local.ShutdownServer(server.ServerConfig)
	}

	id := sessionID(server)

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
		if err = sm.local.ShutdownServer(gmcp.ServerConfig{URL: fmt.Sprintf("http://%s.%s.svc.%s/sse", id, sm.mcpNamespace, sm.mcpClusterDomain), Scope: pods.Items[0].Name}); err != nil {
			return err
		}
	}

	return nil
}

// ShutdownServer will close the connections to the MCP server and remove the Kubernetes objects.
func (sm *SessionManager) ShutdownServer(ctx context.Context, server ServerConfig) error {
	if err := sm.CloseClient(ctx, server); err != nil {
		return err
	}

	id := sessionID(server)

	if sm.client != nil {
		if err := apply.New(sm.client).WithNamespace(sm.mcpNamespace).WithOwnerSubContext(id).WithPruneTypes(new(corev1.Secret), new(appsv1.Deployment), new(corev1.Service)).Apply(ctx, nil, nil); err != nil {
			return fmt.Errorf("failed to delete MCP deployment %s: %w", id, err)
		}
	}
	return nil
}

func (sm *SessionManager) Load(ctx context.Context, tool types.Tool) (result []types.Tool, _ error) {
	_, configData, _ := strings.Cut(tool.Instructions, "\n")

	var servers Config
	if err := json.Unmarshal([]byte(strings.TrimSpace(configData)), &servers); err != nil {
		return nil, fmt.Errorf("failed to parse MCP configuration: %w\n%s", err, configData)
	}

	if len(servers.MCPServers) == 0 {
		// Try to load just one server
		var server ServerConfig
		if err := json.Unmarshal([]byte(strings.TrimSpace(configData)), &server); err != nil {
			return nil, fmt.Errorf("failed to parse single MCP server configuration: %w\n%s", err, configData)
		}
		if server.Command == "" && server.URL == "" && server.Server == "" {
			return nil, fmt.Errorf("no MCP server configuration found in tool instructions: %s", configData)
		}
		servers.MCPServers = map[string]ServerConfig{
			"default": server,
		}
	}

	if len(servers.MCPServers) > 1 {
		return nil, fmt.Errorf("only a single MCP server definition is supported")
	}

	for key, server := range servers.MCPServers {
		config, err := sm.ensureDeployment(ctx, server, key, strings.TrimSuffix(tool.Name, "-bundle"))
		if err != nil {
			return nil, err
		}
		return sm.local.LoadTools(ctx, config, key, tool.Name)
	}

	return nil, fmt.Errorf("no MCP server configuration found in tool instructions: %s", configData)
}

func (sm *SessionManager) KubernetesEnabled() bool {
	return sm.client != nil
}

func (sm *SessionManager) ensureDeployment(ctx context.Context, server ServerConfig, key, serverName string) (gmcp.ServerConfig, error) {
	image := sm.baseImage
	if server.Command == "docker" {
		if len(server.Args) == 0 || !slices.ContainsFunc(sm.allowedDockerImageRepos, func(s string) bool {
			return strings.HasPrefix(server.Args[len(server.Args)-1], s)
		}) {
			return gmcp.ServerConfig{}, fmt.Errorf("docker MCP server must use an image from one of %s", strings.Join(sm.allowedDockerImageRepos, ", "))
		}
		image = server.Args[len(server.Args)-1]
	}

	if server.Command == "" || !sm.KubernetesEnabled() {
		if !sm.allowLocalhostMCP && server.URL != "" {
			// Ensure the URL is not a localhost URL.
			u, err := url.Parse(server.URL)
			if err != nil {
				return gmcp.ServerConfig{}, fmt.Errorf("failed to parse MCP server URL: %w", err)
			}

			// LookupHost will properly detect IP addresses.
			addrs, err := net.DefaultResolver.LookupHost(ctx, u.Hostname())
			if err != nil {
				return gmcp.ServerConfig{}, fmt.Errorf("failed to resolve MCP server URL hostname: %w", err)
			}

			for _, addr := range addrs {
				if ip := net.ParseIP(addr); ip != nil && ip.IsLoopback() {
					return gmcp.ServerConfig{}, fmt.Errorf("MCP server URL must not be a localhost URL: %s", server.URL)
				}
			}
		}
		// Either we aren't deploying to Kubernetes, or this is a URL-based MCP server (so there is nothing to deploy to Kubernetes).
		return server.ServerConfig, nil
	}

	args := []string{"run", "--listen-address", ":8099", "/run/nanobot.yaml"}
	annotations := map[string]string{
		"mcp-server-tool-name":   serverName,
		"mcp-server-config-name": key,
		"mcp-server-scope":       server.Scope,
	}

	id := sessionID(server)
	objs := make([]kclient.Object, 0, 5)

	secretStringData := make(map[string]string, len(server.Env)+len(server.Headers)+2)
	secretVolumeStringData := make(map[string]string, len(server.Files))
	nanobotFileStringData := make(map[string]string, 1)

	for _, file := range server.Files {
		filename := fmt.Sprintf("%s-%s", id, hash.Digest(file))
		secretVolumeStringData[filename] = file.Data
		if file.EnvKey != "" {
			secretStringData[file.EnvKey] = filename
		}
	}

	objs = append(objs, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name.SafeConcatName(id, "files"),
			Namespace:   sm.mcpNamespace,
			Annotations: annotations,
		},
		StringData: secretVolumeStringData,
	})

	for _, env := range server.Env {
		k, v, ok := strings.Cut(env, "=")
		if ok {
			secretStringData[k] = v
		}
	}
	for _, header := range server.Headers {
		k, v, ok := strings.Cut(header, "=")
		if ok {
			secretStringData[k] = v
		}
	}

	var err error
	nanobotFileStringData["nanobot.yaml"], err = constructNanobotYAML(serverName, server.Command, server.Args, secretStringData)
	if err != nil {
		return gmcp.ServerConfig{}, fmt.Errorf("failed to construct nanobot.yaml: %w", err)
	}

	objs = append(objs, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name.SafeConcatName(id, "run"),
			Namespace:   sm.mcpNamespace,
			Annotations: annotations,
		},
		StringData: nanobotFileStringData,
	})

	annotations["obot-revision"] = hash.Digest(hash.Digest(secretStringData) + hash.Digest(secretVolumeStringData))

	// Set an environment variable to indicate that the MCP server is running in Kubernetes.
	// This is something that our special images read and react to.
	secretStringData["OBOT_KUBERNETES_MODE"] = "true"

	// Tell nanobot to expose the healthz endpoint
	secretStringData["NANOBOT_RUN_HEALTHZ_PATH"] = "/healthz"

	objs = append(objs, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name.SafeConcatName(id, "config"),
			Namespace:   sm.mcpNamespace,
			Annotations: annotations,
		},
		StringData: secretStringData,
	})

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        id,
			Namespace:   sm.mcpNamespace,
			Annotations: annotations,
			Labels: map[string]string{
				"app": id,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": id,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels: map[string]string{
						"app": id,
					},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "files",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: name.SafeConcatName(id, "files"),
								},
							},
						},
						{
							Name: "run-file",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: name.SafeConcatName(id, "run"),
								},
							},
						},
					},
					Containers: []corev1.Container{{
						Name:            "mcp",
						Image:           image,
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: 8099,
						}},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &[]bool{false}[0],
							RunAsNonRoot:             &[]bool{true}[0],
							RunAsUser:                &[]int64{1000}[0],
							RunAsGroup:               &[]int64{1000}[0],
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/healthz",
									Port: intstr.FromString("http"),
								},
							},
						},
						Args: args,
						EnvFrom: []corev1.EnvFromSource{{
							SecretRef: &corev1.SecretEnvSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: name.SafeConcatName(id, "config"),
								},
							},
						}},
						VolumeMounts: []corev1.VolumeMount{
							{
								Name:      "files",
								MountPath: "/files",
							},
							{
								Name:      "run-file",
								MountPath: "/run",
							},
						},
					}},
				},
			},
		},
	}
	objs = append(objs, dep)

	objs = append(objs, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        id,
			Namespace:   sm.mcpNamespace,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromString("http"),
				},
			},
			Selector: map[string]string{
				"app": id,
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	})

	if err := apply.New(sm.client).WithNamespace(sm.mcpNamespace).WithOwnerSubContext(id).Apply(ctx, nil, objs...); err != nil {
		return gmcp.ServerConfig{}, fmt.Errorf("failed to create MCP deployment %s: %w", id, err)
	}

	u := fmt.Sprintf("http://%s.%s.svc.%s", id, sm.mcpNamespace, sm.mcpClusterDomain)
	podName, err := sm.updatedMCPPodName(ctx, u, id)
	if err != nil {
		return gmcp.ServerConfig{}, err
	}

	// Use the pod name as the scope, so we get a new session if the pod restarts. MCP sessions aren't persistent on the server side.
	return gmcp.ServerConfig{URL: fmt.Sprintf("%s/sse", u), Scope: podName, AllowedTools: server.AllowedTools}, nil
}

func (sm *SessionManager) transformServerConfig(ctx context.Context, mcpServer v1.MCPServer, serverConfig ServerConfig) (gmcp.ServerConfig, error) {
	serverName := mcpServer.Spec.Manifest.Name
	if serverName == "" {
		serverName = mcpServer.Name
	}

	return sm.ensureDeployment(ctx, serverConfig, "default", serverName)
}

func sessionID(server ServerConfig) string {
	// The allowed tools aren't part of the session ID.
	server.AllowedTools = nil
	return "mcp" + hash.Digest(server)[:60]
}

func (sm *SessionManager) updatedMCPPodName(ctx context.Context, url, id string) (string, error) {
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

	// Not get the pod name that is currently running, waiting for there to only be one pod.
	var pods corev1.PodList
	for {
		if err = sm.client.List(ctx, &pods, &kclient.ListOptions{
			Namespace: sm.mcpNamespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"app": id,
			}),
		}); err != nil {
			return "", fmt.Errorf("failed to list MCP pods: %w", err)
		}

		if len(pods.Items) == 1 && pods.Items[0].Status.Phase == corev1.PodRunning {
			return pods.Items[0].Name, nil
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
