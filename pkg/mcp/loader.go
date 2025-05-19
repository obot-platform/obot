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
	"github.com/obot-platform/obot/pkg/wait"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	local                                     *gmcp.Local
	baseImage, mcpNamespace, mcpClusterDomain string
	allowedDockerImageRepos                   []string
	allowLocalhostMCP                         bool
}

func NewSessionManager(ctx context.Context, defaultLoader *gmcp.Local, opts Options) (*SessionManager, error) {
	var client kclient.WithWatch
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
	}

	return &SessionManager{
		client:                  client,
		local:                   defaultLoader,
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

// ShutdownServer will close the connections to the MCP server and remove the Kubernetes objects.
func (sm *SessionManager) ShutdownServer(ctx context.Context, server ServerConfig) error {
	if sm.client == nil || server.Command == "" {
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
		err = sm.local.ShutdownServer(gmcp.ServerConfig{URL: fmt.Sprintf("http://%s.%s.svc.%s/sse", id, sm.mcpNamespace, sm.mcpClusterDomain), Scope: pods.Items[0].Name})
		if err != nil {
			return err
		}
	}

	if err = apply.New(sm.client).WithNamespace(sm.mcpNamespace).WithOwnerSubContext(id).WithPruneTypes(new(corev1.Secret), new(appsv1.Deployment), new(corev1.Service)).Apply(ctx, nil, nil); err != nil {
		return fmt.Errorf("failed to delete MCP deployment %s: %w", id, err)
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
		if server.Command == "" || sm.client == nil {
			if !sm.allowLocalhostMCP && server.URL != "" {
				// Ensure the URL is not a localhost URL.
				u, err := url.Parse(server.URL)
				if err != nil {
					return nil, fmt.Errorf("failed to parse MCP server URL: %w", err)
				}

				// LookupHost will properly detect IP addresses.
				addrs, err := net.LookupHost(u.Hostname())
				if err != nil {
					return nil, fmt.Errorf("failed to resolve MCP server URL hostname: %w", err)
				}

				for _, addr := range addrs {
					if ip := net.ParseIP(addr); ip != nil && ip.IsLoopback() {
						return nil, fmt.Errorf("MCP server URL must not be a localhost URL: %s", server.URL)
					}
				}
			}
			// Either we aren't deploying to Kubernetes, or this is a URL-based MCP server (so there is nothing to deploy to Kubernetes).
			return sm.local.LoadTools(ctx, server.ServerConfig, tool.Name)
		}

		image := sm.baseImage
		args := []string{"--stdio", fmt.Sprintf("%s %s", server.Command, strings.Join(server.Args, " ")), "--port", "8080", "--healthEndpoint", "/healthz"}
		if server.Command == "docker" {
			if len(server.Args) == 0 || !slices.ContainsFunc(sm.allowedDockerImageRepos, func(s string) bool {
				return strings.HasPrefix(server.Args[0], s)
			}) {
				return nil, fmt.Errorf("docker MCP server must use an image from one of %s", strings.Join(sm.allowedDockerImageRepos, ", "))
			}
			image = server.Args[0]
			args = nil
		}

		annotations := map[string]string{
			"mcp-server-tool-name":   tool.Name,
			"mcp-server-config-name": key,
			"mcp-server-scope":       server.Scope,
		}
		id := sessionID(server)

		var objs []kclient.Object

		secretStringData := make(map[string]string, len(server.Env)+len(server.Headers))
		secretVolumeStringData := make(map[string]string, len(server.Files))
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
						Labels: map[string]string{
							"app": id,
						},
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{{
							Name: "files",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: name.SafeConcatName(id, "files"),
								},
							},
						}},
						Containers: []corev1.Container{{
							Name:            "mcp",
							Image:           image,
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{{
								Name:          "http",
								ContainerPort: 8080,
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
										Port: intstr.FromInt32(8080),
									},
								},
								InitialDelaySeconds: 3,
							},
							Args: args,
							EnvFrom: []corev1.EnvFromSource{{
								SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: name.SafeConcatName(id, "config"),
									},
								},
							}},
							VolumeMounts: []corev1.VolumeMount{{
								Name:      "files",
								MountPath: "/files",
							}},
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
						TargetPort: intstr.FromInt32(8080),
					},
				},
				Selector: map[string]string{
					"app": id,
				},
				Type: corev1.ServiceTypeClusterIP,
			},
		})

		if err := apply.New(sm.client).WithNamespace(sm.mcpNamespace).WithOwnerSubContext(id).Apply(ctx, nil, objs...); err != nil {
			return nil, fmt.Errorf("failed to create MCP deployment %s: %w", id, err)
		}

		url := fmt.Sprintf("http://%s.%s.svc.%s", id, sm.mcpNamespace, sm.mcpClusterDomain)
		podName, err := sm.updatedMCPPodName(ctx, url, id)
		if err != nil {
			return nil, err
		}

		// Use the pod name as the scope, so we get a new session if the pod restarts. MCP sessions aren't persistent on the server side.
		return sm.local.LoadTools(ctx, gmcp.ServerConfig{URL: fmt.Sprintf("%s/sse", url), Scope: podName}, tool.Name)
	}

	return nil, fmt.Errorf("no MCP server configuration found in tool instructions: %s", configData)
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
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timed out waiting for MCP server to be ready")
		case <-time.After(100 * time.Millisecond):
		}

		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
	}

	// Not get the pod name that is currently running, waiting for there to only be one pod.
	var pods corev1.PodList
	for len(pods.Items) != 1 || pods.Items[0].Status.Phase != corev1.PodRunning {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timed out waiting for MCP server to be ready")
		case <-time.After(time.Second):
		}

		if err = sm.client.List(ctx, &pods, &kclient.ListOptions{
			Namespace: sm.mcpNamespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"app": id,
			}),
		}); err != nil {
			return "", fmt.Errorf("failed to list MCP pods: %w", err)
		}
	}

	return pods.Items[0].Name, nil
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
