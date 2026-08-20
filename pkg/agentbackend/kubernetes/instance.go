package kubernetes

import (
	"context"
	"fmt"
	"maps"
	"path"
	"sort"
	"strings"

	"slices"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/obot/pkg/agentbackend"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (b *Backend) ReconcileInstance(ctx context.Context, desired agentbackend.DesiredInstance) (agentbackend.InstanceObservation, error) {
	if desired.Ref.ID == "" {
		return agentbackend.InstanceObservation{}, fmt.Errorf("instance ID is required")
	}
	if desired.Pool.ID == "" {
		return agentbackend.InstanceObservation{}, fmt.Errorf("pool ID is required")
	}
	if desired.Revision == "" {
		return agentbackend.InstanceObservation{}, fmt.Errorf("instance revision is required")
	}
	if desired.Image == "" {
		return agentbackend.InstanceObservation{}, fmt.Errorf("instance image is required")
	}

	// A sandbox cannot outlive its pool, and creating one against a missing or
	// suspended pool would leave a Deployment that can never schedule a pod.
	pool, err := b.ObservePool(ctx, desired.Pool)
	if err != nil {
		return agentbackend.InstanceObservation{}, err
	}
	if !pool.Exists {
		return agentbackend.InstanceObservation{}, fmt.Errorf("pool %s does not exist", desired.Pool.ID)
	}
	if !pool.Schedulable {
		return errorObservation(desired.Ref, "PoolSuspended", "the pool does not admit new sandboxes"), nil
	}

	objs, err := b.instanceObjects(desired)
	if err != nil {
		return agentbackend.InstanceObservation{}, err
	}
	if err := b.applyInstance(ctx, desired.Ref.ID, objs...); err != nil {
		return agentbackend.InstanceObservation{}, err
	}

	return b.ObserveInstance(ctx, desired.Ref)
}

func (b *Backend) ObserveInstance(ctx context.Context, ref agentbackend.InstanceRef) (agentbackend.InstanceObservation, error) {
	if ref.ID == "" {
		return agentbackend.InstanceObservation{}, fmt.Errorf("instance ID is required")
	}

	observation := agentbackend.InstanceObservation{Ref: ref}
	name := instanceName(ref.ID)

	var deployment appsv1.Deployment
	if err := b.cachedClient.Get(ctx, kclient.ObjectKey{Name: name, Namespace: b.opts.Namespace}, &deployment); apierrors.IsNotFound(err) {
		observation.State = agentbackend.StatePending
		return observation, nil
	} else if err != nil {
		return agentbackend.InstanceObservation{}, fmt.Errorf("failed to read sandbox %s: %w", ref.ID, err)
	}

	observation.Ref.BackendID = name
	observation.Exists = true
	// The applied revision is reported regardless of health. Obot compares it
	// against the desired revision to decide whether it still needs to write,
	// and reads State separately to decide whether the sandbox is usable.
	observation.ObservedRevision = deployment.Annotations[revisionAnnotation]
	if deployment.Spec.Template.Annotations[schedulingRevisionAnnotation] != b.podSchedulingRevision() {
		// Scheduling defaults are deployment-wide and are not part of the
		// desired agent revision. Report drift so the controller invokes
		// ReconcileInstance on its next pass.
		observation.ObservedRevision = ""
	}
	observation.BackendGeneration = deployment.Generation
	// An agent with no port serves nothing, so publishing an address for it
	// would offer a link that cannot work.
	if hasServicePort(&deployment) {
		observation.URL = b.instanceURL(name)
	}

	pods, err := b.instancePods(ctx, ref.ID)
	if err != nil {
		return agentbackend.InstanceObservation{}, err
	}

	state, reason, message := classifyDeployment(&deployment, pods)
	observation.State = state
	observation.Reason = reason
	observation.Message = message
	if state != agentbackend.StateReady {
		// Only advertise the URL once something is actually listening on it.
		observation.URL = ""
	}
	return observation, nil
}

// DeleteInstance removes the sandbox and then erases its directory on the
// shared pool volume. kubelet never cleans up a subPath directory, so without
// this every deleted sandbox would leak its workspace.
//
// The cleanup job is created before the workload is pruned, because it is the
// only place the pool identity survives: InstanceRef does not carry the
// pool, and the Deployment that does is about to be deleted.
func (b *Backend) DeleteInstance(ctx context.Context, ref agentbackend.InstanceRef) (agentbackend.DeleteResult, error) {
	if ref.ID == "" {
		return agentbackend.DeleteResult{}, fmt.Errorf("instance ID is required")
	}

	var job batchv1.Job
	err := b.client.Get(ctx, kclient.ObjectKey{Name: cleanupJobName(ref.ID), Namespace: b.opts.Namespace}, &job)
	switch {
	case err == nil:
		return b.finishCleanup(ctx, ref, &job)
	case !apierrors.IsNotFound(err):
		return agentbackend.DeleteResult{}, fmt.Errorf("failed to read cleanup job for sandbox %s: %w", ref.ID, err)
	}

	var deployment appsv1.Deployment
	if err := b.client.Get(ctx, kclient.ObjectKey{Name: instanceName(ref.ID), Namespace: b.opts.Namespace}, &deployment); apierrors.IsNotFound(err) {
		// Nothing left to identify a pool with. Prune anything that survived and
		// treat an already absent sandbox as deleted.
		return agentbackend.DeleteResult{Complete: true}, b.applyInstance(ctx, ref.ID)
	} else if err != nil {
		return agentbackend.DeleteResult{}, fmt.Errorf("failed to read sandbox %s: %w", ref.ID, err)
	}

	poolID := deployment.Labels[poolLabel]
	if poolID == "" {
		return agentbackend.DeleteResult{}, fmt.Errorf("sandbox %s has no pool label; cannot locate its pool volume", ref.ID)
	}

	cleanup, err := b.cleanupJob(ref.ID, poolID)
	if err != nil {
		return agentbackend.DeleteResult{}, err
	}
	if err := b.client.Create(ctx, cleanup); err != nil && !apierrors.IsAlreadyExists(err) {
		return agentbackend.DeleteResult{}, fmt.Errorf("failed to create cleanup job for sandbox %s: %w", ref.ID, err)
	}

	// Stop the agent now that cleanup is recorded, so it is not writing to the
	// directory the job is about to remove.
	if err := b.applyInstance(ctx, ref.ID); err != nil {
		return agentbackend.DeleteResult{}, err
	}
	return agentbackend.DeleteResult{}, nil
}

func (b *Backend) finishCleanup(ctx context.Context, ref agentbackend.InstanceRef, job *batchv1.Job) (agentbackend.DeleteResult, error) {
	for _, condition := range job.Status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case batchv1.JobComplete:
			if err := b.applyInstance(ctx, ref.ID); err != nil {
				return agentbackend.DeleteResult{}, err
			}
			policy := metav1.DeletePropagationBackground
			if err := kclient.IgnoreNotFound(b.client.Delete(ctx, job, &kclient.DeleteOptions{PropagationPolicy: &policy})); err != nil {
				return agentbackend.DeleteResult{}, fmt.Errorf("failed to remove cleanup job for sandbox %s: %w", ref.ID, err)
			}
			return agentbackend.DeleteResult{Complete: true}, nil
		case batchv1.JobFailed:
			// Keep the job so its logs remain available, and let the controller
			// retry. Obot holds the finalizer, so nothing is orphaned meanwhile.
			return agentbackend.DeleteResult{}, fmt.Errorf("cleanup job for sandbox %s failed: %s", ref.ID, condition.Message)
		}
	}
	return agentbackend.DeleteResult{}, nil
}

func (b *Backend) applyInstance(ctx context.Context, instanceID string, objs ...kclient.Object) error {
	err := apply.New(b.client).
		WithNamespace(b.opts.Namespace).
		WithOwnerSubContext("agent-instance-"+sanitize(instanceID)).
		WithPruneTypes(
			new(corev1.Secret),
			new(appsv1.Deployment),
			new(corev1.Service),
		).
		Apply(ctx, nil, objs...)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to apply sandbox objects for instance %s: %w", instanceID, err)
	}
	return nil
}

func (b *Backend) instancePods(ctx context.Context, instanceID string) ([]corev1.Pod, error) {
	var pods corev1.PodList
	if err := b.cachedClient.List(ctx, &pods, kclient.InNamespace(b.opts.Namespace), kclient.MatchingLabels{
		instanceLabel: sanitize(instanceID),
	}); err != nil {
		return nil, fmt.Errorf("failed to list pods for sandbox %s: %w", instanceID, err)
	}
	return pods.Items, nil
}

func (b *Backend) instanceURL(name string) string {
	return fmt.Sprintf("http://%s.%s.svc.%s", name, b.opts.Namespace, b.opts.ClusterDomain)
}

func (b *Backend) instanceObjects(desired agentbackend.DesiredInstance) ([]kclient.Object, error) {
	var (
		name   = instanceName(desired.Ref.ID)
		pool   = poolName(desired.Pool.ID)
		labels = map[string]string{
			managedLabel:  managedValue,
			instanceLabel: sanitize(desired.Ref.ID),
			poolLabel:     sanitize(desired.Pool.ID),
			userLabel:     sanitize(desired.Ref.UserID),
		}
		annotations = map[string]string{revisionAnnotation: desired.Revision}
	)

	// Refused before anything is built: this name is the sandbox's only
	// separation from the rest of the pool's data.
	subdir, err := sandboxSubdir(desired.Ref.ID)
	if err != nil {
		return nil, err
	}

	secretData := make(map[string][]byte, len(desired.Env)+len(desired.Files))
	for key, value := range desired.Env {
		secretData[key] = []byte(value)
	}

	// Secret-backed files ride the same mechanism as rendered files, so a
	// credential lands as a file rather than an environment variable. Agents run
	// model-directed commands and every subprocess inherits the environment,
	// which would expose the credential to everything the agent executes.
	files := slices.Clone(desired.Files)
	for _, ref := range desired.Secrets {
		if ref.FilePath == "" {
			continue
		}
		files = append(files, agentbackend.File{
			Path:    ref.FilePath,
			Content: []byte(ref.Value),
			// Group-readable, not owner-only. Secret volume files are owned by
			// root and the pod's fsGroup, and nothing here runs as root, so
			// 0400 would leave the agent unable to read its own credential.
			// Every process in the pod joins fsGroup as a supplementary group,
			// which is what makes 0440 readable by the agent and no one else.
			Mode: 0o440,
		})
	}

	fileVolumes, fileMounts, err := renderFiles(name, files, secretData)
	if err != nil {
		return nil, err
	}

	env, err := secretRefEnv(desired.Secrets)
	if err != nil {
		return nil, err
	}

	secret := &corev1.Secret{
		Name:        name,
		Namespace:   b.opts.Namespace,
		Labels:      labels,
		Annotations: annotations,
		Data:        secretData,
	}

	volumes := append([]corev1.Volume{{
		Name: workspaceVolumeName,
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: pool,
		},
	}}, fileVolumes...)

	mounts := append([]corev1.VolumeMount{{
		Name:      workspaceVolumeName,
		MountPath: workspaceMountPath,
		// Every sandbox in the pool shares one volume and is separated by
		// subPath alone. This isolates names, not capacity. An empty subPath
		// would mount the pool root, so the name is validated rather than
		// sanitized.
		SubPath: subdir,
	}}, fileMounts...)

	podLabels := make(map[string]string, len(labels))
	maps.Copy(podLabels, labels)
	podAnnotations := maps.Clone(annotations)
	if schedulingRevision := b.podSchedulingRevision(); schedulingRevision != "" {
		podAnnotations[schedulingRevisionAnnotation] = schedulingRevision
	}

	deployment := &appsv1.Deployment{
		Name:        name,
		Namespace:   b.opts.Namespace,
		Labels:      labels,
		Annotations: annotations,
		Spec: appsv1.DeploymentSpec{
			Replicas: new(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{instanceLabel: sanitize(desired.Ref.ID)},
			},
			// The pool volume is ReadWriteOnce and the sandbox owns a subPath
			// within it. A rolling update would briefly run two pods against the
			// same directory.
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: podLabels,
					// Carried on the template as well as the Deployment so a
					// revision change actually replaces the pod.
					Annotations: podAnnotations,
				},
				Spec: corev1.PodSpec{
					// Ties the sandbox to its pool's quota. Every pool shares one
					// priority value, so this expresses accounting, not urgency.
					PriorityClassName: pool,
					SecurityContext:   b.podSecurityContext(),
					Volumes:           volumes,
					Containers: []corev1.Container{{
						Name:            agentContainerName,
						Image:           desired.Image,
						ImagePullPolicy: b.imagePullPolicy(),
						WorkingDir:      workspaceMountPath,
						// The equivalent of `docker run -it`. A shell entrypoint
						// reads EOF on stdin and exits immediately without both of
						// these, which surfaces as CrashLoopBackOff.
						TTY:   desired.Harness.Interactive,
						Stdin: desired.Harness.Interactive,
						Ports: containerPorts(desired.Port),
						Env:   env,
						EnvFrom: []corev1.EnvFromSource{{
							SecretRef: &corev1.SecretEnvSource{
								Name: name,
							},
						}},
						VolumeMounts:    mounts,
						Resources:       containerResources(desired.Requests, desired.Limits),
						SecurityContext: b.containerSecurityContext(),
					}},
				},
			},
		},
	}

	if b.opts.RuntimeClassName != "" {
		deployment.Spec.Template.Spec.RuntimeClassName = new(b.opts.RuntimeClassName)
	}
	b.setPodScheduling(&deployment.Spec.Template.Spec)

	for _, pullSecret := range b.opts.ImagePullSecrets {
		deployment.Spec.Template.Spec.ImagePullSecrets = append(
			deployment.Spec.Template.Spec.ImagePullSecrets,
			corev1.LocalObjectReference{Name: pullSecret},
		)
	}

	objects := []kclient.Object{secret, deployment}

	// An agent that declares no port serves nothing, and a Service with no ports
	// is rejected outright by the API server. Omitting it is therefore not an
	// optimisation: including it makes every port-less agent unreconcilable, and
	// because the Deployment is applied first, the sandbox comes up while its
	// instance never leaves pending.
	if len(servicePorts(desired.Port)) > 0 {
		objects = append(objects, &corev1.Service{
			Name:        name,
			Namespace:   b.opts.Namespace,
			Labels:      labels,
			Annotations: annotations,
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{instanceLabel: sanitize(desired.Ref.ID)},
				Ports:    servicePorts(desired.Port),
			},
		})
	}

	return objects, nil
}

// containerResources makes the sandbox Burstable: it reserves an equal share of
// its pool and may burst to the whole pool when neighbours are idle. Both
// figures are computed from the pool by agentbackend.SandboxShare and carried in
// desired state, so this only translates them.
func containerResources(requests, limits agentbackend.InstanceResources) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    cpuQuantity(requests.CPUVCPUs),
			corev1.ResourceMemory: memoryQuantity(requests.MemoryBytes),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    cpuQuantity(limits.CPUVCPUs),
			corev1.ResourceMemory: memoryQuantity(limits.MemoryBytes),
		},
	}
}

// renderFiles stores each file in the instance secret and mounts it back at its
// absolute path. Files are grouped by directory because a secret volume mounts
// a directory, not a single file.
func renderFiles(secretName string, files []agentbackend.File, secretData map[string][]byte) ([]corev1.Volume, []corev1.VolumeMount, error) {
	if len(files) == 0 {
		return nil, nil, nil
	}

	byDir := map[string][]corev1.KeyToPath{}
	for _, file := range files {
		if !path.IsAbs(file.Path) {
			return nil, nil, fmt.Errorf("file path %q must be absolute", file.Path)
		}
		key := secretKey(file.Path)
		if _, exists := secretData[key]; exists {
			return nil, nil, fmt.Errorf("file path %q collides with another file or environment variable", file.Path)
		}
		secretData[key] = file.Content

		item := corev1.KeyToPath{Key: key, Path: path.Base(file.Path)}
		if file.Mode != 0 {
			item.Mode = new(int32(file.Mode))
		}
		dir := path.Dir(file.Path)
		byDir[dir] = append(byDir[dir], item)
	}

	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	var (
		volumes []corev1.Volume
		mounts  []corev1.VolumeMount
	)
	for i, dir := range dirs {
		items := byDir[dir]
		sort.Slice(items, func(a, b int) bool { return items[a].Path < items[b].Path })
		volumeName := fmt.Sprintf("%s-%d", filesVolumePrefix, i)
		volumes = append(volumes, corev1.Volume{
			Name: volumeName,
			Secret: &corev1.SecretVolumeSource{
				SecretName: secretName,
				Items:      items,
			},
		})
		mounts = append(mounts, corev1.VolumeMount{
			Name:      volumeName,
			MountPath: dir,
			ReadOnly:  true,
		})
	}
	return volumes, mounts, nil
}

// secretKey turns an absolute path into a secret key. Secret keys allow
// alphanumerics, '-', '_' and '.', so the separator is the only substitution
// and the mapping stays one-to-one.
func secretKey(filePath string) string {
	return strings.ReplaceAll(strings.TrimPrefix(filePath, "/"), "/", "_")
}

func secretRefEnv(refs []agentbackend.SecretRef) ([]corev1.EnvVar, error) {
	var env []corev1.EnvVar
	for _, ref := range refs {
		if ref.FilePath != "" {
			// Delivered as a file by instanceObjects.
			continue
		}
		if ref.EnvName == "" {
			return nil, fmt.Errorf("secret %q has neither a file path nor an environment variable name", ref.ID)
		}
		env = append(env, corev1.EnvVar{
			Name: ref.EnvName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					Name: ref.ID,
					Key:  ref.EnvName,
				},
			},
		})
	}
	return env, nil
}

// cleanupScript erases one sandbox's directory from the shared pool volume.
//
// The directory name arrives as an argument rather than interpolated into the
// script, and is re-checked here even though sandboxSubdir has already
// validated it. The command is an rm -rf against a volume shared by every
// sandbox in the pool, so it is worth the cost of refusing to run rather than
// trusting that no future caller reaches this with an empty or traversing name.
const cleanupScript = `set -eu
dir="$1"
case "$dir" in
  ''|.|..|*/*|*..*)
    echo "refusing to remove pool directory: $dir" >&2
    exit 1
    ;;
esac
rm -rf "/pool/$dir"
`

func (b *Backend) cleanupJob(instanceID, poolID string) (*batchv1.Job, error) {
	subdir, err := sandboxSubdir(instanceID)
	if err != nil {
		return nil, err
	}
	job := &batchv1.Job{
		Name:      cleanupJobName(instanceID),
		Namespace: b.opts.Namespace,
		Labels: map[string]string{
			managedLabel:  managedValue,
			instanceLabel: subdir,
			poolLabel:     sanitize(poolID),
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: new(int32(4)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						managedLabel: managedValue,
						poolLabel:    sanitize(poolID),
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					// Runs in the pool, so it lands on the node the pool volume is
					// attached to and counts against the pool's own budget.
					PriorityClassName: poolName(poolID),
					SecurityContext:   b.podSecurityContext(),
					Volumes: []corev1.Volume{{
						Name: workspaceVolumeName,
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: poolName(poolID),
						},
					}},
					Containers: []corev1.Container{{
						Name:    "cleanup",
						Image:   b.opts.CleanupImage,
						Command: []string{"/bin/sh", "-c"},
						// The pool root is mounted rather than the subPath, because
						// the directory itself has to be removed. The subdirectory is
						// passed as $1 so that no part of it is ever parsed as shell.
						Args: []string{cleanupScript, "obot-cleanup", subdir},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      workspaceVolumeName,
							MountPath: "/pool",
						}},
						// Once a quota constrains a resource, every pod in its scope
						// must declare both a request and a limit for it. This job
						// runs in the pool, so omitting them makes the quota reject
						// it forever and deletion never completes.
						Resources:       cleanupResources(),
						SecurityContext: b.containerSecurityContext(),
					}},
				},
			},
		},
	}
	b.setPodScheduling(&job.Spec.Template.Spec)
	return job, nil
}

func (b *Backend) setPodScheduling(spec *corev1.PodSpec) {
	if b.opts.Affinity != nil {
		spec.Affinity = b.opts.Affinity.DeepCopy()
	}
	if len(b.opts.Tolerations) > 0 {
		spec.Tolerations = make([]corev1.Toleration, len(b.opts.Tolerations))
		for i := range b.opts.Tolerations {
			b.opts.Tolerations[i].DeepCopyInto(&spec.Tolerations[i])
		}
	}
	if len(b.opts.NodeSelector) > 0 {
		spec.NodeSelector = maps.Clone(b.opts.NodeSelector)
	}
}

// cleanupResources keeps the job small and Guaranteed. Deleting a sandbox must
// not be blocked by a pool that is close to its budget, so this asks for as
// little as is practical.
func cleanupResources() corev1.ResourceRequirements {
	cpu := cpuQuantity(0.01)
	memory := memoryQuantity(64 << 20)
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory},
		Limits:   corev1.ResourceList{corev1.ResourceCPU: cpu, corev1.ResourceMemory: memory},
	}
}

// hasServicePort reports whether the sandbox declares an HTTP port, read back
// from the applied Deployment so observation does not need desired state.
func hasServicePort(deployment *appsv1.Deployment) bool {
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if len(container.Ports) > 0 {
			return true
		}
	}
	return false
}

// containerPorts declares the agent's HTTP port. An agent that serves nothing
// declares no port, so nothing suggests it is reachable.
func containerPorts(port int) []corev1.ContainerPort {
	if port <= 0 {
		return nil
	}
	return []corev1.ContainerPort{{Name: "http", ContainerPort: int32(port)}}
}

func servicePorts(port int) []corev1.ServicePort {
	if port <= 0 {
		return nil
	}
	return []corev1.ServicePort{{
		Name:       "http",
		Port:       80,
		TargetPort: intstr.FromString("http"),
	}}
}

func errorObservation(ref agentbackend.InstanceRef, reason, message string) agentbackend.InstanceObservation {
	return agentbackend.InstanceObservation{
		Ref:     ref,
		State:   agentbackend.StateError,
		Reason:  reason,
		Message: message,
	}
}

// imagePullPolicy resolves the configured policy, defaulting to Always so a
// mutable tag is re-pulled on every start.
func (b *Backend) imagePullPolicy() corev1.PullPolicy {
	switch corev1.PullPolicy(b.opts.ImagePullPolicy) {
	case corev1.PullIfNotPresent:
		return corev1.PullIfNotPresent
	case corev1.PullNever:
		return corev1.PullNever
	default:
		return corev1.PullAlways
	}
}
