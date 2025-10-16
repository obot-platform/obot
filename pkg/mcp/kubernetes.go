package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
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
	clientset        *kubernetes.Clientset
	client           kclient.WithWatch
	baseImage        string
	mcpNamespace     string
	mcpClusterDomain string
	imagePullSecrets []string
}

func newKubernetesBackend(clientset *kubernetes.Clientset, client kclient.WithWatch, baseImage, mcpNamespace, mcpClusterDomain string, imagePullSecrets []string) backend {
	return &kubernetesBackend{
		clientset:        clientset,
		client:           client,
		baseImage:        baseImage,
		mcpNamespace:     mcpNamespace,
		mcpClusterDomain: mcpClusterDomain,
		imagePullSecrets: imagePullSecrets,
	}
}

func (k *kubernetesBackend) ensureServerDeployment(ctx context.Context, server ServerConfig, id, mcpServerDisplayName, mcpServerName string) (ServerConfig, error) {
	switch server.Runtime {
	case types.RuntimeNPX, types.RuntimeUVX, types.RuntimeContainerized:
	default:
		return ServerConfig{}, fmt.Errorf("unsupported MCP runtime: %s", server.Runtime)
	}

	// Generate the Kubernetes deployment objects.
	objs, err := k.k8sObjects(id, server, mcpServerDisplayName, mcpServerName)
	if err != nil {
		return ServerConfig{}, fmt.Errorf("failed to generate kubernetes objects for server %s: %w", id, err)
	}

	if err := apply.New(k.client).WithNamespace(k.mcpNamespace).WithOwnerSubContext(id).Apply(ctx, nil, objs...); err != nil {
		return ServerConfig{}, fmt.Errorf("failed to create MCP deployment %s: %w", id, err)
	}

	u := fmt.Sprintf("http://%s.%s.svc.%s", id, k.mcpNamespace, k.mcpClusterDomain)
	podName, err := k.updatedMCPPodName(ctx, u, id, server)
	if err != nil {
		return ServerConfig{}, err
	}

	fullURL := fmt.Sprintf("%s/%s", u, strings.TrimPrefix(server.ContainerPath, "/"))

	// Use the pod name as the scope, so we get a new session if the pod restarts. MCP sessions aren't persistent on the server side.
	return ServerConfig{URL: fullURL, Scope: podName, AllowedTools: server.AllowedTools}, nil
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
	}).Stream(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get logs: %w", err)
	}

	return logs, nil
}

func (k *kubernetesBackend) transformConfig(ctx context.Context, id string, serverConfig ServerConfig) (*ServerConfig, error) {
	var pods corev1.PodList
	if err := k.client.List(ctx, &pods, &kclient.ListOptions{
		Namespace: k.mcpNamespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"app": id,
		}),
	}); err != nil {
		return nil, fmt.Errorf("failed to list MCP pods: %w", err)
	} else if len(pods.Items) == 0 {
		// If the pod was removed, then this won't do anything. The session will only get cleaned up when the server restarts.
		// That's better than the alternative of having unusable sessions that users are still trying to use.
		return nil, nil
	}

	return &ServerConfig{URL: fmt.Sprintf("http://%s.%s.svc.%s/%s", id, k.mcpNamespace, k.mcpClusterDomain, strings.TrimPrefix(serverConfig.ContainerPath, "/")), Scope: pods.Items[0].Name}, nil
}

func (k *kubernetesBackend) shutdownServer(ctx context.Context, id string) error {
	if err := apply.New(k.client).WithNamespace(k.mcpNamespace).WithOwnerSubContext(id).WithPruneTypes(new(corev1.Secret), new(appsv1.Deployment), new(corev1.Service)).Apply(ctx, nil, nil); err != nil {
		return fmt.Errorf("failed to delete MCP deployment %s: %w", id, err)
	}

	return nil
}

func (k *kubernetesBackend) k8sObjects(id string, server ServerConfig, serverDisplayName, serverName string) ([]kclient.Object, error) {
	var (
		command []string
		objs    = make([]kclient.Object, 0, 5)
		image   = k.baseImage
		args    = []string{"run", "--listen-address", fmt.Sprintf(":%d", defaultContainerPort), "/run/nanobot.yaml"}
		port    = 8099

		annotations = map[string]string{
			"mcp-server-name":  serverName,
			"mcp-server-scope": server.Scope,
		}

		fileMapping            = make(map[string]string, len(server.Files))
		secretStringData       = make(map[string]string, len(server.Env)+len(server.Headers)+3)
		secretVolumeStringData = make(map[string]string, len(server.Files))
		metaEnv                = make([]string, 0, len(server.Env)+len(server.Headers)+len(server.Files))
	)

	for _, file := range server.Files {
		filename := fmt.Sprintf("%s-%s", id, hash.Digest(file))
		secretVolumeStringData[filename] = file.Data
		if file.EnvKey != "" {
			metaEnv = append(metaEnv, file.EnvKey)
			secretStringData[file.EnvKey] = "/files/" + filename
			fileMapping[file.EnvKey] = "/files/" + filename
		}
	}

	objs = append(objs, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name.SafeConcatName(id, "files"),
			Namespace:   k.mcpNamespace,
			Annotations: annotations,
		},
		StringData: secretVolumeStringData,
	})

	for _, env := range server.Env {
		k, v, ok := strings.Cut(env, "=")
		if ok {
			metaEnv = append(metaEnv, k)
			secretStringData[k] = v
		}
	}
	for _, header := range server.Headers {
		k, v, ok := strings.Cut(header, "=")
		if ok {
			metaEnv = append(metaEnv, k)
			secretStringData[k] = v
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

	if server.Runtime == types.RuntimeContainerized {
		if server.Command != "" {
			command = []string{expandEnvVars(server.Command, fileMapping, nil)}
		}

		image = expandEnvVars(server.ContainerImage, fileMapping, nil)
		args = server.Args
		port = server.ContainerPort
	} else {
		nanobotFileString, err := constructNanobotYAML(serverDisplayName, server.Command, server.Args, secretStringData)
		if err != nil {
			return nil, fmt.Errorf("failed to construct nanobot.yaml: %w", err)
		}

		objs = append(objs, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:        name.SafeConcatName(id, "run"),
				Namespace:   k.mcpNamespace,
				Annotations: annotations,
			},
			StringData: map[string]string{
				"nanobot.yaml": nanobotFileString,
			},
		})
	}

	annotations["obot-revision"] = hash.Digest(hash.Digest(secretStringData) + hash.Digest(secretVolumeStringData))

	// Set this environment variable for our nanobot image to read
	secretStringData["NANOBOT_META_ENV"] = strings.Join(metaEnv, ",")

	// Set an environment variable to indicate that the MCP server is running in Kubernetes.
	// This is something that our special images read and react to.
	secretStringData["OBOT_KUBERNETES_MODE"] = "true"

	// Tell nanobot to expose the healthz endpoint
	secretStringData["NANOBOT_RUN_HEALTHZ_PATH"] = "/healthz"

	objs = append(objs, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name.SafeConcatName(id, "config"),
			Namespace:   k.mcpNamespace,
			Annotations: annotations,
		},
		StringData: secretStringData,
	})

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        id,
			Namespace:   k.mcpNamespace,
			Annotations: annotations,
			Labels: map[string]string{
				"app":             id,
				"mcp-server-name": serverName,
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
						"app":             id,
						"mcp-server-name": serverName,
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
							ContainerPort: int32(port),
						}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("400Mi"),
							},
						},
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

	if server.Runtime != types.RuntimeContainerized {
		dep.Spec.Template.Spec.Containers[0].VolumeMounts = append(dep.Spec.Template.Spec.Containers[0].VolumeMounts, corev1.VolumeMount{
			Name:      "run-file",
			MountPath: "/run",
			ReadOnly:  true,
		})

		dep.Spec.Template.Spec.Containers[0].ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.FromString("http"),
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
			Name:        id,
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
				"app": id,
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
				if p.Status.Phase == corev1.PodRunning {
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

	patch := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
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

	if err := k.client.Patch(ctx, &deployment, kclient.RawPatch(ktypes.MergePatchType, patchBytes)); err != nil {
		return fmt.Errorf("failed to patch deployment %s: %w", id, err)
	}

	return nil
}
