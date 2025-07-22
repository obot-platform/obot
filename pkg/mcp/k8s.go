package mcp

import (
	"fmt"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/name"
	otypes "github.com/obot-platform/obot/apiclient/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (sm *SessionManager) k8sObjectsForUVXOrNPX(server ServerConfig, key, serverName string) ([]kclient.Object, error) {
	if server.Runtime != otypes.RuntimeUVX && server.Runtime != otypes.RuntimeNPX {
		return nil, fmt.Errorf("invalid runtime: %s", server.Runtime)
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
		return nil, fmt.Errorf("failed to construct nanobot.yaml: %w", err)
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
						Image:           sm.baseImage,
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

	return objs, nil
}

func (sm *SessionManager) k8sObjectsForContainerized(server ServerConfig, key, serverName string) ([]kclient.Object, error) {
	if server.Runtime != otypes.RuntimeContainerized {
		return nil, fmt.Errorf("invalid runtime: %s", server.Runtime)
	}

	annotations := map[string]string{
		"mcp-server-tool-name":   serverName,
		"mcp-server-config-name": key,
		"mcp-server-scope":       server.Scope,
	}

	id := sessionID(server)
	objs := make([]kclient.Object, 0, 4)

	secretStringData := make(map[string]string, len(server.Env)+len(server.Headers)+2)
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

	annotations["obot-revision"] = hash.Digest(hash.Digest(secretStringData) + hash.Digest(secretVolumeStringData))

	objs = append(objs, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name.SafeConcatName(id, "config"),
			Namespace:   sm.mcpNamespace,
			Annotations: annotations,
		},
		StringData: secretStringData,
	})

	var command []string
	if server.Command != "" {
		command = []string{server.Command}
	}

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
					},
					Containers: []corev1.Container{{
						Name:            "mcp",
						Image:           server.ContainerImage,
						ImagePullPolicy: corev1.PullAlways,
						Ports: []corev1.ContainerPort{{
							Name:          "http",
							ContainerPort: int32(server.ContainerPort),
						}},
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: &[]bool{false}[0],
							RunAsNonRoot:             &[]bool{true}[0],
							RunAsUser:                &[]int64{1000}[0],
							RunAsGroup:               &[]int64{1000}[0],
						},
						Args:    server.Args,
						Command: command,
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

	return objs, nil
}
