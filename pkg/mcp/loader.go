package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	gmcp "github.com/gptscript-ai/gptscript/pkg/mcp"
	"github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/pkg/wait"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const mcpNamespace = "obot-mcp"

type SessionManager struct {
	client                      kclient.WithWatch
	local                       *gmcp.Local
	baseImage, mcpClusterDomain string
}

func NewSessionManager(ctx context.Context, baseImage, mcpClusterDomain string) (*SessionManager, error) {
	config, err := buildConfig()
	if err != nil {
		return nil, err
	}

	client, err := kclient.NewWithWatch(config, kclient.Options{})
	if err != nil {
		return nil, err
	}

	if err = kclient.IgnoreAlreadyExists(client.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: mcpNamespace,
		},
	})); err != nil {
		return nil, err
	}

	return &SessionManager{
		client:           client,
		local:            new(gmcp.Local),
		baseImage:        baseImage,
		mcpClusterDomain: mcpClusterDomain,
	}, nil
}

// Close does nothing with the deployments and services. It just closes the local session.
func (sm *SessionManager) Close() error {
	return sm.local.Close()
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

	for _, server := range servers.MCPServers {
		if server.Command == "" {
			// This is a URL-based MCP server, so we don't have to do any deployments.
			return sm.local.LoadTools(ctx, server.ServerConfig, tool.Name)
		}

		id := "mcp" + hash.Digest(server)[:60]

		var objs []kclient.Object

		secretStringData := make(map[string]string, len(server.Env)+len(server.Headers)+4)
		secretVolumeStringData := make(map[string]string, len(server.Files))
		for _, file := range server.Files {
			name := fmt.Sprintf("%s-%s", id, hash.Digest(file))
			secretVolumeStringData[name] = file.Data
			if file.EnvKey != "" {
				secretStringData[file.EnvKey] = name
			}
		}

		objs = append(objs, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name.SafeConcatName(id, "files"),
				Namespace: mcpNamespace,
			},
			StringData: secretVolumeStringData,
		})

		secretStringData["SERVER"] = server.Server
		secretStringData["URL"] = server.URL
		secretStringData["COMMAND"] = server.Command
		secretStringData["BASE_URL"] = server.BaseURL
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
				Name:      id,
				Namespace: mcpNamespace,
			},
			StringData: secretStringData,
		})

		var args []string
		if server.Command != "" {
			args = make([]string, 0, len(server.Args)+1)
			args = append(args, server.Command)
			args = append(args, server.Args...)
		}

		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      id,
				Namespace: mcpNamespace,
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
									SecretName: id,
								},
							},
						}},
						Containers: []corev1.Container{{
							Name:  "mcp",
							Image: sm.baseImage,
							Ports: []corev1.ContainerPort{{
								Name:          "http",
								ContainerPort: 80,
							}},
							Args: args,
							EnvFrom: []corev1.EnvFromSource{{
								SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: id,
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
				Name:      id,
				Namespace: mcpNamespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{
					{
						Name: "http",
						Port: 80,
					},
				},
				Selector: map[string]string{
					"app": id,
				},
				Type: corev1.ServiceTypeClusterIP,
			},
		})

		for _, dep := range objs {
			if err := sm.client.Get(ctx, kclient.ObjectKey{Name: id, Namespace: mcpNamespace}, dep); apierrors.IsNotFound(err) {
				if err = sm.client.Create(ctx, dep); err != nil {
					return nil, fmt.Errorf("failed to create %T: %w", dep, err)
				}
			} else if err != nil {
				return nil, fmt.Errorf("failed to check for %T: %w", dep, err)
			}
		}

		if _, err := wait.For(ctx, sm.client, dep, func(dep *appsv1.Deployment) (bool, error) {
			return dep.Status.UpdatedReplicas > 0 && dep.Status.ReadyReplicas > 0, nil
		}); err != nil {
			return nil, fmt.Errorf("failed to wait for deployment %s: %w", id, err)
		}

		return sm.local.LoadTools(ctx, gmcp.ServerConfig{URL: fmt.Sprintf("%s.%s.svc.%s", id, mcpNamespace, sm.mcpClusterDomain)}, tool.Name)
	}

	return nil, fmt.Errorf("no MCP server configuration found in tool instructions: %s", configData)
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
