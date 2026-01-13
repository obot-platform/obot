package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var olog = logger.Package()

type kubernetesBackend struct {
	clientset                     *kubernetes.Clientset
	client                        kclient.WithWatch
	baseImage                     string
	httpWebhookBaseImage          string
	remoteShimBaseImage           string
	mcpNamespace                  string
	mcpClusterDomain              string
	serviceFQDN                   string
	imagePullSecrets              []string
	auditLogsBatchSize            int
	auditLogsFlushIntervalSeconds int
	obotClient                    kclient.Client
}

func newKubernetesBackend(clientset *kubernetes.Clientset, client kclient.WithWatch, obotClient kclient.Client, opts Options) backend {
	var serviceFQDN string
	if opts.ServiceName != "" && opts.ServiceNamespace != "" {
		serviceFQDN = fmt.Sprintf("%s.%s.svc.%s", opts.ServiceName, opts.ServiceNamespace, opts.MCPClusterDomain)
	}

	return &kubernetesBackend{
		clientset:                     clientset,
		client:                        client,
		baseImage:                     opts.MCPBaseImage,
		httpWebhookBaseImage:          opts.MCPHTTPWebhookBaseImage,
		remoteShimBaseImage:           opts.MCPRemoteShimBaseImage,
		mcpNamespace:                  opts.MCPNamespace,
		mcpClusterDomain:              opts.MCPClusterDomain,
		serviceFQDN:                   serviceFQDN,
		imagePullSecrets:              opts.MCPImagePullSecrets,
		auditLogsBatchSize:            opts.MCPAuditLogsPersistBatchSize,
		auditLogsFlushIntervalSeconds: opts.MCPAuditLogPersistIntervalSeconds,
		obotClient:                    obotClient,
	}
}

func (k *kubernetesBackend) deployServer(ctx context.Context, server ServerConfig, webhooks []Webhook) error {
	// Check capacity before deploying (fail-open if capacity can't be determined)
	if err := k.CheckCapacity(ctx); err != nil {
		return err
	}

	// Generate the Kubernetes deployment objects.
	objs, err := k.k8sObjects(ctx, server, webhooks)
	if err != nil {
		return fmt.Errorf("failed to generate kubernetes objects for server %s: %w", server.MCPServerName, err)
	}

	if err := apply.New(k.client).WithNamespace(k.mcpNamespace).WithOwnerSubContext(server.Scope).WithPruneTypes(new(corev1.Secret), new(appsv1.Deployment), new(corev1.Service)).Apply(ctx, nil, nil); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to cleanup old MCP deployment %s: %w", server.MCPServerName, err)
	}

	if err := apply.New(k.client).WithNamespace(k.mcpNamespace).WithOwnerSubContext(server.MCPServerName).Apply(ctx, nil, objs...); err != nil {
		return fmt.Errorf("failed to create MCP deployment %s: %w", server.MCPServerName, err)
	}

	return nil
}

func (k *kubernetesBackend) ensureServerDeployment(ctx context.Context, server ServerConfig, webhooks []Webhook) (ServerConfig, error) {
	// Transform component URLs to use internal service FQDN
	for i, component := range server.Components {
		component.URL = k.replaceHostWithServiceFQDN(component.URL)
		server.Components[i] = component
	}

	if err := k.deployServer(ctx, server, webhooks); err != nil {
		return ServerConfig{}, err
	}

	u := fmt.Sprintf("http://%s.%s.svc.%s", server.MCPServerName, k.mcpNamespace, k.mcpClusterDomain)
	podName, err := k.updatedMCPPodName(ctx, u, server.MCPServerName, server)
	if err != nil {
		return ServerConfig{}, err
	}

	fullURL := fmt.Sprintf("%s/%s", u, strings.TrimPrefix(server.ContainerPath, "/"))

	// Use the pod name as the scope, so we get a new session if the pod restarts. MCP sessions aren't persistent on the server side.
	return ServerConfig{
		URL:                  fullURL,
		MCPServerName:        server.MCPServerName,
		Audiences:            server.Audiences,
		MCPServerNamespace:   server.MCPServerNamespace,
		MCPServerDisplayName: server.MCPServerDisplayName,
		Scope:                podName,
		UserID:               server.UserID,
		Runtime:              types.RuntimeRemote,
		Issuer:               server.Issuer,
		ContainerPort:        server.ContainerPort,
		ContainerPath:        server.ContainerPath,
	}, nil
}

func (k *kubernetesBackend) getServerDetails(ctx context.Context, id string) (types.MCPServerDetails, error) {
	var deployment appsv1.Deployment
	if err := k.client.Get(ctx, kclient.ObjectKey{Name: id, Namespace: k.mcpNamespace}, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return types.MCPServerDetails{}, fmt.Errorf("mcp server %s is not running", id)
		}

		return types.MCPServerDetails{}, fmt.Errorf("failed to get deployment %s: %w", id, err)
	}

	var (
		lastRestart types.Time
		pods        corev1.PodList
		podEvents   []corev1.Event
	)
	if err := k.client.List(ctx, &pods, kclient.InNamespace(k.mcpNamespace), kclient.MatchingLabels(deployment.Spec.Selector.MatchLabels)); err != nil {
		return types.MCPServerDetails{}, fmt.Errorf("failed to get pods: %w", err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			lastRestart = types.Time{Time: pod.CreationTimestamp.Time}
		}

		var eventList corev1.EventList
		if err := k.client.List(ctx, &eventList, kclient.InNamespace(k.mcpNamespace), kclient.MatchingFieldsSelector{
			Selector: fields.SelectorFromSet(map[string]string{
				"involvedObject.kind":      "Pod",
				"involvedObject.name":      pod.Name,
				"involvedObject.namespace": pod.Namespace,
			}),
		}); err != nil {
			return types.MCPServerDetails{}, fmt.Errorf("failed to get events: %w", err)
		}

		podEvents = append(podEvents, eventList.Items...)
	}

	var deploymentEvents corev1.EventList
	if err := k.client.List(ctx, &deploymentEvents, kclient.InNamespace(k.mcpNamespace), kclient.MatchingFieldsSelector{
		Selector: fields.SelectorFromSet(map[string]string{
			"involvedObject.kind":      "Deployment",
			"involvedObject.name":      deployment.Name,
			"involvedObject.namespace": deployment.Namespace,
		}),
	}); err != nil {
		return types.MCPServerDetails{}, fmt.Errorf("failed to get events: %w", err)
	}

	allEvents := append(deploymentEvents.Items, podEvents...)
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].CreationTimestamp.Before(&allEvents[j].CreationTimestamp)
	})

	var mcpEvents []types.MCPServerEvent
	for _, event := range allEvents {
		mcpEvents = append(mcpEvents, types.MCPServerEvent{
			Time:         types.Time{Time: event.CreationTimestamp.Time},
			Reason:       event.Reason,
			Message:      event.Message,
			EventType:    event.Type,
			Action:       event.Action,
			Count:        event.Count,
			ResourceName: event.InvolvedObject.Name,
			ResourceKind: event.InvolvedObject.Kind,
		})
	}

	return types.MCPServerDetails{
		DeploymentName: deployment.Name,
		Namespace:      deployment.Namespace,
		LastRestart:    lastRestart,
		ReadyReplicas:  deployment.Status.ReadyReplicas,
		Replicas:       deployment.Status.Replicas,
		IsAvailable:    deployment.Status.ReadyReplicas > 0,
		Events:         mcpEvents,
	}, nil
}

func (k *kubernetesBackend) streamServerLogs(ctx context.Context, id string) (io.ReadCloser, error) {
	var deployment appsv1.Deployment
	if err := k.client.Get(ctx, kclient.ObjectKey{Name: id, Namespace: k.mcpNamespace}, &deployment); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("mcp server %s is not running", id)
		}

		return nil, fmt.Errorf("failed to get deployment %s: %w", id, err)
	}

	var pods corev1.PodList
	if err := k.client.List(ctx, &pods, kclient.InNamespace(k.mcpNamespace), kclient.MatchingLabels(deployment.Spec.Selector.MatchLabels)); err != nil {
		return nil, fmt.Errorf("failed to get pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no pods found for deployment %s", id)
	}

	tailLines := int64(100)
	logs, err := k.clientset.CoreV1().Pods(k.mcpNamespace).GetLogs(pods.Items[0].Name, &corev1.PodLogOptions{
		Follow:     true,
		Timestamps: true,
		TailLines:  &tailLines,
		Container:  "mcp",
	}).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	return logs, nil
}

func (k *kubernetesBackend) transformConfig(ctx context.Context, serverConfig ServerConfig) (*ServerConfig, error) {
	var pods corev1.PodList
	if err := k.client.List(ctx, &pods, &kclient.ListOptions{
		Namespace: k.mcpNamespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app": serverConfig.MCPServerName,
		}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list MCP pods: %w", err)
	} else if len(pods.Items) == 0 {
		// If the pod was removed, then this won't do anything. The session will only get cleaned up when the server restarts.
		// That's better than the alternative of having unusable sessions that users are still trying to use.
		return nil, nil
	}

	return &ServerConfig{URL: fmt.Sprintf("http://%s.%s.svc.%s/%s", serverConfig.MCPServerName, k.mcpNamespace, k.mcpClusterDomain, strings.TrimPrefix(serverConfig.ContainerPath, "/")), MCPServerName: pods.Items[0].Name}, nil
}

// replaceHostWithServiceFQDN replaces the host and port in a URL with the internal service FQDN.
func (k *kubernetesBackend) replaceHostWithServiceFQDN(urlStr string) string {
	if k.serviceFQDN == "" || urlStr == "" {
		return urlStr
	}

	// Parse the URL to extract the path
	idx := strings.Index(urlStr, "://")
	if idx == -1 {
		return urlStr
	}

	rest := urlStr[idx+3:]

	// Find where the path starts (after host:port)
	pathIdx := strings.Index(rest, "/")
	var path string
	if pathIdx != -1 {
		path = rest[pathIdx:]
	}

	// Reconstruct URL with service FQDN
	return fmt.Sprintf("http://%s%s", k.serviceFQDN, path)
}

func (k *kubernetesBackend) shutdownServer(ctx context.Context, id string) error {
	if err := apply.New(k.client).WithNamespace(k.mcpNamespace).WithOwnerSubContext(id).WithPruneTypes(new(corev1.Secret), new(appsv1.Deployment), new(corev1.Service)).Apply(ctx, nil, nil); err != nil {
		return fmt.Errorf("failed to delete MCP deployment %s: %w", id, err)
	}

	return nil
}

func (k *kubernetesBackend) k8sObjects(ctx context.Context, server ServerConfig, webhooks []Webhook) ([]kclient.Object, error) {
	var (
		command  []string
		objs     = make([]kclient.Object, 0, 5)
		image    = k.baseImage
		args     = []string{"run", "--disable-ui", "--listen-address", fmt.Sprintf(":%d", defaultContainerPort), "/run/nanobot.yaml"}
		port     = defaultContainerPort
		portName = "http"

		annotations = map[string]string{
			"mcp-server-display-name": server.MCPServerDisplayName,
			"mcp-server-scope":        server.MCPServerName,
			"mcp-user-id":             server.UserID,
		}

		fileMapping            = make(map[string]string, len(server.Files))
		secretEnvStringData    = make(map[string]string, len(server.Env)+10)
		secretVolumeStringData = make(map[string]string, len(server.Files))
		headerData             = make(map[string]string, len(server.Headers))
		metaEnv                = make([]string, 0, len(server.Env)+len(server.Files))
	)

	// Use remote shim image for remote runtimes
	switch server.Runtime {
	case types.RuntimeRemote, types.RuntimeComposite:
		image = k.remoteShimBaseImage
	case types.RuntimeContainerized:
		port = server.ContainerPort
	}

	for _, file := range server.Files {
		filename := fmt.Sprintf("%s-%s", server.MCPServerName, hash.Digest(file))
		secretVolumeStringData[filename] = file.Data
		if file.EnvKey != "" {
			metaEnv = append(metaEnv, file.EnvKey)
			secretEnvStringData[file.EnvKey] = "/files/" + filename
			fileMapping[file.EnvKey] = "/files/" + filename
		}
	}

	objs = append(objs, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name.SafeConcatName(server.MCPServerName, "files"),
			Namespace:   k.mcpNamespace,
			Annotations: annotations,
		},
		StringData: secretVolumeStringData,
	})

	for _, env := range server.Env {
		k, v, ok := strings.Cut(env, "=")
		if ok {
			metaEnv = append(metaEnv, k)
			secretEnvStringData[k] = v
		}
	}
	for _, header := range server.Headers {
		k, v, ok := strings.Cut(header, "=")
		if ok {
			headerData[k] = v
		}
	}

	if len(server.Args) > 0 {
		// Copy the args to avoid modifying the original slice.
		args := make([]string, len(server.Args))
		for i, arg := range server.Args {
			args[i] = expandEnvVars(arg, fileMapping, nil)
		}

		server.Args = args
	}

	// Set this environment variable for our nanobot image to read
	secretEnvStringData["NANOBOT_META_ENV"] = strings.Join(metaEnv, ",")

	// Set an environment variable to indicate that the MCP server is running in Kubernetes.
	// This is something that our special images read and react to.
	secretEnvStringData["OBOT_KUBERNETES_MODE"] = "true"

	// Set an environment variable to force fetch tool list
	secretEnvStringData["NANOBOT_RUN_FORCE_FETCH_TOOL_LIST"] = "true"

	// Tell nanobot to expose the healthz endpoint
	secretEnvStringData["NANOBOT_RUN_HEALTHZ_PATH"] = "/healthz"

	// JWT environment variables
	secretEnvStringData["NANOBOT_RUN_OAUTH_SCOPES"] = "profile"
	secretEnvStringData["NANOBOT_RUN_TRUSTED_ISSUER"] = server.Issuer
	secretEnvStringData["NANOBOT_RUN_OAUTH_JWKSURL"] = k.replaceHostWithServiceFQDN(server.JWKSEndpoint)
	secretEnvStringData["NANOBOT_RUN_TRUSTED_AUDIENCES"] = strings.Join(server.Audiences, ",")
	secretEnvStringData["NANOBOT_RUN_OAUTH_CLIENT_ID"] = server.TokenExchangeClientID
	secretEnvStringData["NANOBOT_RUN_OAUTH_CLIENT_SECRET"] = server.TokenExchangeClientSecret
	secretEnvStringData["NANOBOT_RUN_OAUTH_TOKEN_URL"] = k.replaceHostWithServiceFQDN(server.TokenExchangeEndpoint)
	secretEnvStringData["NANOBOT_RUN_OAUTH_AUTHORIZE_URL"] = k.replaceHostWithServiceFQDN(server.AuthorizeEndpoint)
	secretEnvStringData["NANOBOT_DISABLE_HEALTH_CHECKER"] = strconv.FormatBool(server.Runtime == types.RuntimeRemote || server.Runtime == types.RuntimeComposite)
	// Audit log variables
	secretEnvStringData["NANOBOT_RUN_AUDIT_LOG_TOKEN"] = server.AuditLogToken
	secretEnvStringData["NANOBOT_RUN_AUDIT_LOG_SEND_URL"] = k.replaceHostWithServiceFQDN(server.AuditLogEndpoint)
	secretEnvStringData["NANOBOT_RUN_AUDIT_LOG_BATCH_SIZE"] = strconv.Itoa(k.auditLogsBatchSize)
	secretEnvStringData["NANOBOT_RUN_AUDIT_LOG_FLUSH_INTERVAL_SECONDS"] = strconv.Itoa(k.auditLogsFlushIntervalSeconds)
	secretEnvStringData["NANOBOT_RUN_AUDIT_LOG_METADATA"] = server.AuditLogMetadata
	// API key authentication webhook URL
	secretEnvStringData["NANOBOT_RUN_APIKEY_AUTH_WEBHOOK_URL"] = k.replaceHostWithServiceFQDN(server.Issuer + "/api/api-keys/auth")
	secretEnvStringData["NANOBOT_RUN_MCPSERVER_ID"] = strings.TrimSuffix(server.MCPServerName, "-shim")

	annotations["obot-revision"] = hash.Digest(hash.Digest(secretEnvStringData) + hash.Digest(secretVolumeStringData) + hash.Digest(webhooks))

	// Fetch K8s settings
	k8sSettings, err := k.getK8sSettings(ctx)
	if err != nil {
		// Log error but continue with defaults
		log.Warnf("Failed to get K8s settings, using defaults: %v", err)
		k8sSettings = v1.K8sSettingsSpec{}
	}

	// Add K8s settings hash to annotations
	annotations["obot.ai/k8s-settings-hash"] = ComputeK8sSettingsHash(k8sSettings)

	webhookSecretStringData := make(map[string]string, len(webhooks))
	containers := make([]corev1.Container, 0, len(webhooks)+2)
	// Add a container for each webhook, ensuring that there are no port collisions.
	for i, webhook := range webhooks {
		port := port + i + 1
		c, err := webhookToServerConfig(webhook, k.httpWebhookBaseImage, server.MCPServerName, server.UserID, server.Scope, port)
		if err != nil {
			return nil, fmt.Errorf("failed to translate webhook to config %s: %v", webhook.Name, err)
		}

		env := make([]corev1.EnvVar, 0, len(c.Env))
		for _, e := range c.Env {
			key, val, ok := strings.Cut(e, "=")
			if !ok {
				continue
			}

			if key != "WEBHOOK_SECRET" {
				env = append(env, corev1.EnvVar{
					Name:  key,
					Value: val,
				})
			} else {
				secretKey := strings.ToUpper(server.MCPServerName + "_" + key)
				webhookSecretStringData[secretKey] = val
				env = append(env, corev1.EnvVar{
					Name: key,
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{
								Name: name.SafeConcatName(server.MCPServerName, "webhook", "secrets"),
							},
							Key: secretKey,
						},
					},
				})
			}
		}

		containers = append(containers, corev1.Container{
			Name:            c.MCPServerName,
			Image:           k.httpWebhookBaseImage,
			ImagePullPolicy: corev1.PullAlways,
			Ports: []corev1.ContainerPort{{
				ContainerPort: int32(port),
			}},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &[]bool{false}[0],
				RunAsNonRoot:             &[]bool{true}[0],
				RunAsUser:                &[]int64{1000}[0],
				RunAsGroup:               &[]int64{1000}[0],
			},
			Env: env,
		})

		// Update the URL for this webhook for use inside the "main" container.
		webhook.URL = fmt.Sprintf("http://localhost:%d%s", port, c.ContainerPath)
		webhooks[i] = webhook
	}

	objs = append(objs, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name.SafeConcatName(server.MCPServerName, "webhook", "secrets"),
			Namespace:   k.mcpNamespace,
			Annotations: annotations,
		},
		StringData: webhookSecretStringData,
	})

	if server.Runtime != types.RuntimeRemote {
		// If this is anything other than a remote runtime, then we need to add a special shim container.
		// The remote runtime will just be the shim and is deployed as the "real" container.
		nanobotFileString, err := constructNanobotYAMLForServer(server.MCPServerDisplayName+" Shim", fmt.Sprintf("http://localhost:%d/%s", port, strings.TrimPrefix(server.ContainerPath, "/")), "", nil, nil, nil, webhooks)
		if err != nil {
			return nil, fmt.Errorf("failed to construct nanobot.yaml: %w", err)
		}

		annotations["nanobot-file-rev"] = hash.Digest(nanobotFileString)

		objs = append(objs, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name.SafeConcatName(server.MCPServerName, "run", "shim"),
				Namespace:   k.mcpNamespace,
				Annotations: annotations,
			},
			StringData: map[string]string{
				"nanobot.yaml": nanobotFileString,
			},
		})

		objs = append(objs, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name.SafeConcatName(server.MCPServerName, "config", "shim"),
				Namespace:   k.mcpNamespace,
				Annotations: annotations,
			},
			StringData: func() map[string]string {
				vars := make(map[string]string, 15)
				for k, v := range secretEnvStringData {
					if k == "NANOBOT_DISABLE_HEALTH_CHECKER" {
						vars[k] = "true"
						if server.Runtime != types.RuntimeComposite {
							delete(secretEnvStringData, k)
						}
					} else if strings.HasPrefix(k, "NANOBOT_RUN_") {
						vars[k] = v
						if strings.HasPrefix(k, "NANOBOT_RUN_AUDIT_LOG_") || k != "NANOBOT_RUN_HEALTHZ_PATH" && server.Runtime != types.RuntimeComposite {
							delete(secretEnvStringData, k)
						}
					}
				}

				return vars
			}(),
		})

		port := port + len(webhooks) + 1

		containers = append(containers, corev1.Container{
			Name:            server.MCPServerName + "-shim",
			Image:           k.remoteShimBaseImage,
			ImagePullPolicy: corev1.PullAlways,
			Ports: []corev1.ContainerPort{{
				Name:          portName,
				ContainerPort: int32(port),
			}},
			SecurityContext: &corev1.SecurityContext{
				AllowPrivilegeEscalation: &[]bool{false}[0],
				RunAsNonRoot:             &[]bool{true}[0],
				RunAsUser:                &[]int64{1000}[0],
				RunAsGroup:               &[]int64{1000}[0],
			},
			Args: []string{"run", "--disable-ui", "--listen-address", fmt.Sprintf(":%d", port), "/run/nanobot.yaml"},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "run-shim-file",
					MountPath: "/run",
					ReadOnly:  true,
				},
			},
			EnvFrom: []corev1.EnvFromSource{{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name.SafeConcatName(server.MCPServerName, "config", "shim"),
					},
				},
			}},
			ReadinessProbe: &corev1.Probe{
				ProbeHandler: corev1.ProbeHandler{
					HTTPGet: &corev1.HTTPGetAction{
						Path: "/healthz",
						Port: intstr.FromInt(port),
					},
				},
			},
		})

		// Reset the portName so that the service points to the shim.
		portName = ""
		// Remove the webhooks because those are in the shim.
		webhooks = nil

		if server.Runtime == types.RuntimeContainerized {
			if server.Command != "" {
				command = []string{expandEnvVars(server.Command, fileMapping, nil)}
			}

			if server.ContainerImage != "" {
				image = expandEnvVars(server.ContainerImage, fileMapping, nil)
			}

			if server.Args != nil {
				args = server.Args
			}
		}
	}

	objs = append(objs, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name.SafeConcatName(server.MCPServerName, "config"),
			Namespace:   k.mcpNamespace,
			Annotations: annotations,
		},
		StringData: secretEnvStringData,
	})

	// This is the "real" MCP container.
	containers = append(containers, corev1.Container{
		Name:            "mcp",
		Image:           image,
		ImagePullPolicy: corev1.PullAlways,
		Ports: []corev1.ContainerPort{{
			Name:          portName,
			ContainerPort: int32(port),
		}},
		// Apply resources from K8s settings with fallback to default
		Resources: func() corev1.ResourceRequirements {
			if k8sSettings.Resources != nil {
				return *k8sSettings.Resources
			}
			return corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("400Mi"),
				},
			}
		}(),
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: &[]bool{false}[0],
			RunAsNonRoot:             &[]bool{true}[0],
			RunAsUser:                &[]int64{1000}[0],
			RunAsGroup:               &[]int64{1000}[0],
		},
		Command: command,
		Args:    args,
		EnvFrom: []corev1.EnvFromSource{{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: name.SafeConcatName(server.MCPServerName, "config"),
				},
			},
		}},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "files",
				MountPath: "/files",
			},
		},
	})

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        server.MCPServerName,
			Namespace:   k.mcpNamespace,
			Annotations: annotations,
			Labels: map[string]string{
				"app":         server.MCPServerName,
				"mcp-user-id": server.UserID,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": server.MCPServerName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: annotations,
					Labels: map[string]string{
						"app":         server.MCPServerName,
						"mcp-user-id": server.UserID,
					},
				},
				Spec: corev1.PodSpec{
					Affinity:    k8sSettings.Affinity,
					Tolerations: k8sSettings.Tolerations,
					Volumes: []corev1.Volume{
						{
							Name: "files",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: name.SafeConcatName(server.MCPServerName, "files"),
								},
							},
						},
						{
							Name: "run-file",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: name.SafeConcatName(server.MCPServerName, "run"),
								},
							},
						},
						{
							Name: "run-shim-file",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: name.SafeConcatName(server.MCPServerName, "run", "shim"),
								},
							},
						},
					},
					Containers: containers,
				},
			},
		},
	}

	if server.Runtime != types.RuntimeContainerized {
		// Setup the nanobot config file and add it to the last container in the deployment.
		var nanobotFileString string
		if server.Runtime == types.RuntimeComposite {
			nanobotFileString, err = constructNanobotYAMLForCompositeServer(server.Components)
			annotations["nanobot-composite-file-rev"] = hash.Digest(nanobotFileString)
		} else {
			nanobotFileString, err = constructNanobotYAMLForServer(server.MCPServerDisplayName, server.URL, server.Command, server.Args, secretEnvStringData, headerData, webhooks)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to construct nanobot.yaml: %w", err)
		}

		objs = append(objs, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name.SafeConcatName(server.MCPServerName, "run"),
				Namespace:   k.mcpNamespace,
				Annotations: annotations,
			},
			StringData: map[string]string{
				"nanobot.yaml": nanobotFileString,
			},
		})

		dep.Spec.Template.Spec.Containers[len(containers)-1].VolumeMounts = append(dep.Spec.Template.Spec.Containers[len(containers)-1].VolumeMounts, corev1.VolumeMount{
			Name:      "run-file",
			MountPath: "/run",
			ReadOnly:  true,
		})

		dep.Spec.Template.Spec.Containers[len(containers)-1].ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromInt(port),
				},
			},
		}
	}

	if len(k.imagePullSecrets) > 0 {
		for _, secret := range k.imagePullSecrets {
			dep.Spec.Template.Spec.ImagePullSecrets = append(dep.Spec.Template.Spec.ImagePullSecrets, corev1.LocalObjectReference{Name: secret})
		}
	}

	objs = append(objs, dep)

	objs = append(objs, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        server.MCPServerName,
			Namespace:   k.mcpNamespace,
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
				"app": server.MCPServerName,
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	})

	return objs, nil
}

// getNewestPod finds and returns the most recently created pod from the list.
func getNewestPod(pods []corev1.Pod) (*corev1.Pod, error) {
	if len(pods) == 0 {
		return nil, fmt.Errorf("no pods provided")
	}

	newest := &pods[0]
	for i := range pods {
		if pods[i].CreationTimestamp.After(newest.CreationTimestamp.Time) {
			newest = &pods[i]
		}
	}

	return newest, nil
}

// analyzePodStatus examines a pod's status to determine if we should retry waiting for it
// or if we should fail immediately. Returns (shouldRetry, error).
func analyzePodStatus(pod *corev1.Pod) (bool, error) {
	// Check pod phase first
	switch pod.Status.Phase {
	case corev1.PodFailed:
		return false, fmt.Errorf("%w: pod is in Failed phase: %s", ErrHealthCheckTimeout, pod.Status.Message)
	case corev1.PodSucceeded:
		// This shouldn't happen for a long-running deployment, but if it does, it's an error
		return false, fmt.Errorf("%w: pod succeeded and exited", ErrHealthCheckTimeout)
	case corev1.PodUnknown:
		return false, fmt.Errorf("%w: pod is in Unknown phase", ErrHealthCheckTimeout)
	}

	// Check pod conditions for scheduling issues
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
			// Pod can't be scheduled - check if it's a transient issue
			if cond.Reason == corev1.PodReasonUnschedulable {
				// Unschedulable could be transient (e.g., waiting for autoscaler)
				return true, fmt.Errorf("%w: pod unschedulable: %s", ErrPodSchedulingFailed, cond.Message)
			}
		}
	}

	for _, cs := range pod.Status.ContainerStatuses {
		// Check if container is waiting
		if cs.State.Waiting != nil {
			waiting := cs.State.Waiting
			switch waiting.Reason {
			// Transient/recoverable states - should retry
			case "ContainerCreating", "PodInitializing":
				return true, fmt.Errorf("container %s is %s", cs.Name, waiting.Reason)

			// Image pull states - need to check if it's temporary or permanent
			case "ImagePullBackOff", "ErrImagePull":
				// ImagePullBackOff can be transient (network issues) but also permanent (bad image)
				// We'll treat it as retryable for now, but it will eventually hit max retries
				return true, fmt.Errorf("%w: container %s: %s - %s", ErrImagePullFailed, cs.Name, waiting.Reason, waiting.Message)

			// Permanent failures - should not retry
			case "CrashLoopBackOff":
				return false, fmt.Errorf("%w: container %s is in CrashLoopBackOff: %s", ErrPodCrashLoopBackOff, cs.Name, waiting.Message)
			case "InvalidImageName":
				return false, fmt.Errorf("%w: container %s has invalid image name: %s", ErrImagePullFailed, cs.Name, waiting.Message)
			case "CreateContainerConfigError", "CreateContainerError":
				return false, fmt.Errorf("%w: container %s failed to create: %s - %s", ErrPodConfigurationFailed, cs.Name, waiting.Reason, waiting.Message)
			case "RunContainerError":
				return false, fmt.Errorf("%w: container %s failed to run: %s", ErrPodConfigurationFailed, cs.Name, waiting.Message)
			}
		}

		// Check if container terminated with errors and has high restart count
		if cs.State.Terminated != nil && cs.State.Terminated.ExitCode != 0 {
			if cs.RestartCount > 3 {
				return false, fmt.Errorf("%w: container %s repeatedly crashing (exit code %d, %d restarts): %s",
					ErrPodCrashLoopBackOff, cs.Name, cs.State.Terminated.ExitCode, cs.RestartCount, cs.State.Terminated.Reason)
			}
		}
	}

	// Check if pod is being evicted
	if pod.Status.Reason == "Evicted" {
		return false, fmt.Errorf("%w: pod was evicted: %s", ErrPodSchedulingFailed, pod.Status.Message)
	}

	// Default: pod is in Pending or Running but not ready yet - should retry
	return true, fmt.Errorf("pod in phase %s, waiting for containers to be ready", pod.Status.Phase)
}

func (k *kubernetesBackend) updatedMCPPodName(ctx context.Context, url, id string, server ServerConfig) (string, error) {
	const maxRetries = 5
	var lastErr error

	// The Kubernetes backend is always going to have a Nanobot pod running. So, ensure that the runtime is "remote" instead of "containerized"
	server.Runtime = types.RuntimeRemote

	// Retry loop with smart pod status checking
	for attempt := range maxRetries {
		// Wait for the deployment to be updated.
		_, err := wait.For(ctx, k.client, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: id, Namespace: k.mcpNamespace}}, func(dep *appsv1.Deployment) (bool, error) {
			return dep.Generation == dep.Status.ObservedGeneration && dep.Status.Replicas == 1 && dep.Status.UpdatedReplicas == 1 && dep.Status.ReadyReplicas == 1 && dep.Status.AvailableReplicas == 1, nil
		}, wait.Option{Timeout: time.Minute})
		if err == nil {
			// Deployment is ready, now ensure the server is ready
			if err = ensureServerReady(ctx, url, server); err != nil {
				return "", fmt.Errorf("failed to ensure MCP server is ready: %w", err)
			}

			// Now get the pod name that is currently running
			var (
				pods            corev1.PodList
				runningPodCount int
				podName         string
			)
			if err = k.client.List(ctx, &pods, &kclient.ListOptions{
				Namespace: k.mcpNamespace,
				LabelSelector: labels.SelectorFromSet(map[string]string{
					"app": id,
				}),
			}); err != nil {
				return "", fmt.Errorf("failed to list MCP pods: %w", err)
			}

			for _, p := range pods.Items {
				if p.DeletionTimestamp.IsZero() && p.Status.Phase == corev1.PodRunning {
					podName = p.Name
					runningPodCount++
				}
			}

			// runningPodCount should always equal 1, if the deployment is ready, as it is by this point in the code.
			// However, we will check just to make sure, and retry if it isn't.
			if runningPodCount == 1 {
				return podName, nil
			} else if runningPodCount > 1 {
				lastErr = fmt.Errorf("more than one running pod found")
			} else {
				lastErr = fmt.Errorf("no pods found")
			}
			continue
		}

		// Deployment wait timed out, check pod status to decide if we should retry
		var pods corev1.PodList
		if listErr := k.client.List(ctx, &pods, &kclient.ListOptions{
			Namespace: k.mcpNamespace,
			LabelSelector: labels.SelectorFromSet(map[string]string{
				"app": id,
			}),
		}); listErr != nil {
			olog.Debugf("failed to list MCP pods for status check: id=%s error=%v", id, listErr)
			return "", fmt.Errorf("failed to list MCP pods: %w", listErr)
		}

		if len(pods.Items) == 0 {
			olog.Debugf("no pods found for MCP server: id=%s attempt=%d", id, attempt+1)
			lastErr = fmt.Errorf("no pods found")
			if attempt < maxRetries {
				continue
			}
			return "", fmt.Errorf("%w: %v", ErrHealthCheckTimeout, lastErr)
		}

		// Get the newest pod and analyze its status
		newestPod, err := getNewestPod(pods.Items)
		if err != nil {
			olog.Debugf("failed to get newest pod: id=%s error=%v attempt=%d", id, err, attempt+1)
			lastErr = err
			if attempt < maxRetries {
				continue
			}
			return "", fmt.Errorf("%w: %v", ErrHealthCheckTimeout, lastErr)
		}

		shouldRetry, podErr := analyzePodStatus(newestPod)
		lastErr = podErr

		if !shouldRetry {
			// Permanent failure - return the error with the appropriate type already wrapped
			olog.Debugf("pod in non-retryable state: id=%s error=%v attempt=%d", id, podErr, attempt+1)
			return "", podErr
		}
	}

	olog.Debugf("exceeded max retries waiting for pod: id=%s lastError=%v attempts=%d", id, lastErr, maxRetries)
	return "", fmt.Errorf("%w after %d retries: %v", ErrHealthCheckTimeout, maxRetries, lastErr)
}

func (k *kubernetesBackend) restartServer(ctx context.Context, id string) error {
	var deployment appsv1.Deployment
	if err := k.client.Get(ctx, kclient.ObjectKey{Name: id, Namespace: k.mcpNamespace}, &deployment); err != nil {
		return fmt.Errorf("failed to get deployment %s: %w", id, err)
	}

	// Fetch K8s settings
	k8sSettings, err := k.getK8sSettings(ctx)
	if err != nil {
		// Log error but continue with defaults
		log.Warnf("Failed to get K8s settings, using defaults: %v", err)
		k8sSettings = v1.K8sSettingsSpec{}
	}

	// Compute K8s settings hash
	k8sSettingsHash := ComputeK8sSettingsHash(k8sSettings)

	// Build the patch with restart annotation and k8s settings hash
	podAnnotations := map[string]string{
		"kubectl.kubernetes.io/restartedAt": time.Now().Format(time.RFC3339),
		"obot.ai/k8s-settings-hash":         k8sSettingsHash,
	}

	// Update the deployment metadata annotation as well
	deploymentAnnotations := map[string]string{
		"obot.ai/k8s-settings-hash": k8sSettingsHash,
	}

	// Build the patch structure
	templateSpec := make(map[string]any)
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": deploymentAnnotations,
		},
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": podAnnotations,
				},
				"spec": templateSpec,
			},
		},
	}

	// Add affinity if present
	if k8sSettings.Affinity != nil {
		// Use $patch: replace to completely replace the affinity field
		// rather than merging with existing values
		affinityMap := map[string]any{
			"$patch": "replace",
		}

		// Set the actual affinity fields that are present
		if k8sSettings.Affinity.NodeAffinity != nil {
			affinityMap["nodeAffinity"] = k8sSettings.Affinity.NodeAffinity
		}
		if k8sSettings.Affinity.PodAffinity != nil {
			affinityMap["podAffinity"] = k8sSettings.Affinity.PodAffinity
		}
		if k8sSettings.Affinity.PodAntiAffinity != nil {
			affinityMap["podAntiAffinity"] = k8sSettings.Affinity.PodAntiAffinity
		}

		templateSpec["affinity"] = affinityMap
	} else {
		// Use $patch: delete to remove any existing affinity
		templateSpec["affinity"] = map[string]any{
			"$patch": "delete",
		}
	}

	// Add tolerations if present
	if len(k8sSettings.Tolerations) > 0 {
		// For tolerations (an array), setting the value directly will replace the entire array
		templateSpec["tolerations"] = k8sSettings.Tolerations
	} else {
		// Use $patch: delete to remove any existing tolerations
		templateSpec["tolerations"] = map[string]any{
			"$patch": "delete",
		}
	}

	// Add resources to the container
	if k8sSettings.Resources != nil {
		// Use $patch: replace to completely replace the resources field
		resourcesMap := map[string]any{
			"$patch": "replace",
		}

		// Set the actual resource fields that are present
		if len(k8sSettings.Resources.Limits) > 0 {
			resourcesMap["limits"] = k8sSettings.Resources.Limits
		}
		if len(k8sSettings.Resources.Requests) > 0 {
			resourcesMap["requests"] = k8sSettings.Resources.Requests
		}

		// Patch the container resources (container name is "mcp")
		// Using strategic merge patch which can merge containers by name
		templateSpec["containers"] = []map[string]any{
			{
				"name":      "mcp",
				"resources": resourcesMap,
			},
		}
	} else {
		// Use $patch: delete to remove any existing resources
		templateSpec["containers"] = []map[string]any{
			{
				"name": "mcp",
				"resources": map[string]any{
					"$patch": "delete",
				},
			},
		}
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	// Use StrategicMergePatchType to merge containers by name without requiring all fields
	if err := k.client.Patch(ctx, &deployment, kclient.RawPatch(ktypes.StrategicMergePatchType, patchBytes)); err != nil {
		return fmt.Errorf("failed to patch deployment %s: %w", id, err)
	}

	return nil
}

// ComputeK8sSettingsHash computes a hash of K8s settings for change detection
func ComputeK8sSettingsHash(settings v1.K8sSettingsSpec) string {
	var buf bytes.Buffer

	// Hash affinity
	if settings.Affinity != nil {
		affinityJSON, _ := json.Marshal(settings.Affinity)
		buf.Write(affinityJSON)
	}

	// Hash tolerations
	if len(settings.Tolerations) > 0 {
		tolerationsJSON, _ := json.Marshal(settings.Tolerations)
		buf.Write(tolerationsJSON)
	}

	// Hash resources
	if settings.Resources != nil {
		resourcesJSON, _ := json.Marshal(settings.Resources)
		buf.Write(resourcesJSON)
	}

	if buf.Len() == 0 {
		return "none"
	}

	return hash.Digest(buf.String())
}

func (k *kubernetesBackend) getK8sSettings(ctx context.Context) (v1.K8sSettingsSpec, error) {
	var settings v1.K8sSettings
	err := k.obotClient.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.K8sSettingsName,
	}, &settings)

	return settings.Spec, err
}

// CheckCapacity checks if there's enough capacity to deploy a new MCP server.
// Returns nil if capacity is available, or ErrInsufficientCapacity if not.
// Uses fail-open strategy: if no ResourceQuota exists, allows deployment and lets Kubernetes decide.
// Only ResourceQuota is used for precheck since node capacity checks are naive and don't account
// for taints, affinity, other namespace workloads, or resource fragmentation.
func (k *kubernetesBackend) CheckCapacity(ctx context.Context) error {
	// Get the resource requests from K8s settings (defaults: 400Mi memory, 10m CPU)
	memoryRequest := resource.MustParse("400Mi")
	cpuRequest := resource.MustParse("10m")
	k8sSettings, err := k.getK8sSettings(ctx)
	if err == nil && k8sSettings.Resources != nil && k8sSettings.Resources.Requests != nil {
		if mem, ok := k8sSettings.Resources.Requests[corev1.ResourceMemory]; ok {
			memoryRequest = mem
		}
		if cpu, ok := k8sSettings.Resources.Requests[corev1.ResourceCPU]; ok {
			cpuRequest = cpu
		}
	}

	// Only use ResourceQuota for precheck - it's enforced at admission time and accurate
	if available, err := k.checkResourceQuotaCapacity(ctx, memoryRequest, cpuRequest); err == nil {
		if !available {
			return ErrInsufficientCapacity
		}
		return nil
	}

	// No ResourceQuota or can't check - fail open, let Kubernetes decide
	return nil
}

// checkResourceQuotaCapacity checks if there's enough capacity based on ResourceQuota.
// Returns (true, nil) if capacity is available, (false, nil) if not, or (false, error) if quota can't be checked.
func (k *kubernetesBackend) checkResourceQuotaCapacity(ctx context.Context, memoryRequest, cpuRequest resource.Quantity) (bool, error) {
	quotas, err := k.clientset.CoreV1().ResourceQuotas(k.mcpNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list resource quotas: %w", err)
	}

	if len(quotas.Items) == 0 {
		return false, fmt.Errorf("no resource quotas found")
	}

	// Check if any quota has memory or CPU request limits
	for _, quota := range quotas.Items {
		// Check memory
		memHard, hasMemHard := quota.Status.Hard[corev1.ResourceRequestsMemory]
		memUsed, hasMemUsed := quota.Status.Used[corev1.ResourceRequestsMemory]

		if hasMemHard && hasMemUsed {
			available := memHard.DeepCopy()
			available.Sub(memUsed)
			if available.Cmp(memoryRequest) < 0 {
				return false, nil
			}
		}

		// Check CPU
		cpuHard, hasCPUHard := quota.Status.Hard[corev1.ResourceRequestsCPU]
		cpuUsed, hasCPUUsed := quota.Status.Used[corev1.ResourceRequestsCPU]

		if hasCPUHard && hasCPUUsed {
			available := cpuHard.DeepCopy()
			available.Sub(cpuUsed)
			if available.Cmp(cpuRequest) < 0 {
				return false, nil
			}
		}

		// If we found at least one resource limit, we can make a decision
		if (hasMemHard && hasMemUsed) || (hasCPUHard && hasCPUUsed) {
			return true, nil
		}
	}

	return false, fmt.Errorf("no memory or CPU quota found")
}

// GetCapacityInfo returns capacity information for the MCP namespace.
// Used by the admin capacity endpoint.
func (k *kubernetesBackend) GetCapacityInfo(ctx context.Context) types.MCPCapacityInfo {
	// Try ResourceQuota first - this is the only accurate source
	if info, ok := k.getResourceQuotaCapacity(ctx); ok {
		return info
	}

	// Fallback to deployment aggregation only (no limits, just totals)
	// Node metrics are intentionally not used because they don't account for
	// taints, affinity, or other scheduling constraints.
	return k.getDeploymentCapacity(ctx)
}

func (k *kubernetesBackend) getResourceQuotaCapacity(ctx context.Context) (types.MCPCapacityInfo, bool) {
	quotas, err := k.clientset.CoreV1().ResourceQuotas(k.mcpNamespace).List(ctx, metav1.ListOptions{})
	if err != nil || len(quotas.Items) == 0 {
		return types.MCPCapacityInfo{}, false
	}

	info := types.MCPCapacityInfo{
		Source: types.CapacitySourceResourceQuota,
	}

	// Aggregate limits from all ResourceQuotas
	var totalCPULimit, totalMemoryLimit resource.Quantity
	for _, quota := range quotas.Items {
		if hard, ok := quota.Status.Hard[corev1.ResourceRequestsCPU]; ok {
			totalCPULimit.Add(hard)
		}
		if hard, ok := quota.Status.Hard[corev1.ResourceRequestsMemory]; ok {
			totalMemoryLimit.Add(hard)
		}
	}
	info.CPULimit = formatCPU(totalCPULimit)
	info.MemoryLimit = formatMemory(totalMemoryLimit)

	// Calculate requested resources directly from deployments for immediate updates
	// ResourceQuota.Status.Used updates asynchronously and can lag behind actual state
	deployments, err := k.clientset.AppsV1().Deployments(k.mcpNamespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		var totalCPU, totalMemory resource.Quantity
		for _, deployment := range deployments.Items {
			replicas := int64(1)
			if deployment.Spec.Replicas != nil {
				replicas = int64(*deployment.Spec.Replicas)
			}
			for _, container := range deployment.Spec.Template.Spec.Containers {
				if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
					scaled := cpu.DeepCopy()
					scaled.SetMilli(scaled.MilliValue() * replicas)
					totalCPU.Add(scaled)
				}
				if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
					scaled := mem.DeepCopy()
					scaled.Set(scaled.Value() * replicas)
					totalMemory.Add(scaled)
				}
			}
		}
		info.CPURequested = formatCPU(totalCPU)
		info.MemoryRequested = formatMemory(totalMemory)
		info.ActiveDeployments = len(deployments.Items)
	} else {
		// Fallback to ResourceQuota status if deployment list fails
		var totalCPUUsed, totalMemoryUsed resource.Quantity
		for _, quota := range quotas.Items {
			if used, ok := quota.Status.Used[corev1.ResourceRequestsCPU]; ok {
				totalCPUUsed.Add(used)
			}
			if used, ok := quota.Status.Used[corev1.ResourceRequestsMemory]; ok {
				totalMemoryUsed.Add(used)
			}
		}
		info.CPURequested = formatCPU(totalCPUUsed)
		info.MemoryRequested = formatMemory(totalMemoryUsed)
		info.ActiveDeployments = k.countActiveDeployments(ctx)
	}

	return info, true
}

func (k *kubernetesBackend) getDeploymentCapacity(ctx context.Context) types.MCPCapacityInfo {
	info := types.MCPCapacityInfo{
		Source: types.CapacitySourceDeployments,
	}

	deployments, err := k.clientset.AppsV1().Deployments(k.mcpNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		info.Error = "failed to list deployments"
		return info
	}

	var totalCPU, totalMemory resource.Quantity
	for _, deployment := range deployments.Items {
		replicas := int64(1)
		if deployment.Spec.Replicas != nil {
			replicas = int64(*deployment.Spec.Replicas)
		}
		for _, container := range deployment.Spec.Template.Spec.Containers {
			if cpu, ok := container.Resources.Requests[corev1.ResourceCPU]; ok {
				scaled := cpu.DeepCopy()
				scaled.SetMilli(scaled.MilliValue() * replicas)
				totalCPU.Add(scaled)
			}
			if mem, ok := container.Resources.Requests[corev1.ResourceMemory]; ok {
				scaled := mem.DeepCopy()
				scaled.Set(scaled.Value() * replicas)
				totalMemory.Add(scaled)
			}
		}
	}

	info.CPURequested = formatCPU(totalCPU)
	info.MemoryRequested = formatMemory(totalMemory)
	info.ActiveDeployments = len(deployments.Items)

	return info
}

func (k *kubernetesBackend) countActiveDeployments(ctx context.Context) int {
	deployments, err := k.clientset.AppsV1().Deployments(k.mcpNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0
	}
	return len(deployments.Items)
}

// formatCPU formats a CPU quantity in a human-readable format.
// Returns empty string for zero values.
func formatCPU(q resource.Quantity) string {
	if q.IsZero() {
		return ""
	}
	// CPU is typically in millicores, convert to cores if >= 1 core
	millis := q.MilliValue()
	if millis >= 1000 {
		cores := float64(millis) / 1000
		if cores == float64(int64(cores)) {
			return fmt.Sprintf("%d", int64(cores))
		}
		return fmt.Sprintf("%.1f", cores)
	}
	return fmt.Sprintf("%dm", millis)
}

// formatMemory formats a memory quantity in a human-readable format.
// Returns empty string for zero values.
func formatMemory(q resource.Quantity) string {
	if q.IsZero() {
		return ""
	}
	bytes := q.Value()

	const (
		ki = 1024
		mi = ki * 1024
		gi = mi * 1024
		ti = gi * 1024
	)

	switch {
	case bytes >= ti:
		return fmt.Sprintf("%.1fTi", float64(bytes)/float64(ti))
	case bytes >= gi:
		val := float64(bytes) / float64(gi)
		if val == float64(int64(val)) {
			return fmt.Sprintf("%dGi", int64(val))
		}
		return fmt.Sprintf("%.1fGi", val)
	case bytes >= mi:
		val := float64(bytes) / float64(mi)
		if val == float64(int64(val)) {
			return fmt.Sprintf("%dMi", int64(val))
		}
		return fmt.Sprintf("%.1fMi", val)
	case bytes >= ki:
		return fmt.Sprintf("%dKi", bytes/ki)
	default:
		return fmt.Sprintf("%d", bytes)
	}
}
