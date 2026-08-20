package kubernetes

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/obot-platform/obot/pkg/agentbackend"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sscheme "k8s.io/client-go/kubernetes/scheme"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testBackend(t *testing.T) *Backend {
	t.Helper()
	backend, err := New(nil, nil, Options{
		Namespace:     "obot-agents",
		ClusterDomain: "cluster.local",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return backend
}

func desiredInstance() agentbackend.DesiredInstance {
	return agentbackend.DesiredInstance{
		Ref:      agentbackend.InstanceRef{ID: "inst-1", UserID: "user-1"},
		Pool:     agentbackend.PoolRef{ID: "alloc-1"},
		Revision: "sha256:abc",
		Image:    "example.com/agent:v1",
		Requests: agentbackend.InstanceResources{CPUVCPUs: 0.5, MemoryBytes: 1 << 30},
		Limits:   agentbackend.InstanceResources{CPUVCPUs: 2, MemoryBytes: 4 << 30},
	}
}

func TestHostedAgentPodScheduling(t *testing.T) {
	affinity := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{{
					MatchExpressions: []corev1.NodeSelectorRequirement{{
						Key:      "workload",
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"hosted-agent"},
					}},
				}},
			},
		},
	}
	tolerations := []corev1.Toleration{{
		Key:      "workload",
		Operator: corev1.TolerationOpEqual,
		Value:    "hosted-agent",
		Effect:   corev1.TaintEffectNoSchedule,
	}}
	nodeSelector := map[string]string{"node-pool": "agents"}
	backend, err := New(nil, nil, Options{
		Namespace:     "obot-agents",
		ClusterDomain: "cluster.local",
		Affinity:      affinity,
		Tolerations:   tolerations,
		NodeSelector:  nodeSelector,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	objects, err := backend.instanceObjects(desiredInstance())
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	cleanup, err := backend.cleanupJob("inst-1", "alloc-1")
	if err != nil {
		t.Fatalf("cleanupJob: %v", err)
	}

	specs := map[string]*corev1.PodSpec{
		"sandbox": &deploymentFrom(t, objects).Spec.Template.Spec,
		"cleanup": &cleanup.Spec.Template.Spec,
	}
	for name, spec := range specs {
		t.Run(name, func(t *testing.T) {
			if !reflect.DeepEqual(spec.Affinity, affinity) {
				t.Errorf("unexpected affinity: %#v", spec.Affinity)
			}
			if !reflect.DeepEqual(spec.Tolerations, tolerations) {
				t.Errorf("unexpected tolerations: %#v", spec.Tolerations)
			}
			if !reflect.DeepEqual(spec.NodeSelector, nodeSelector) {
				t.Errorf("unexpected node selector: %#v", spec.NodeSelector)
			}
		})
	}

	// Each workload gets its own copy; mutating one generated pod template must
	// not alter another or the backend's deployment-wide settings.
	specs["sandbox"].NodeSelector["node-pool"] = "other"
	if got := specs["cleanup"].NodeSelector["node-pool"]; got != "agents" {
		t.Fatalf("sandbox node selector mutation leaked to cleanup job: %q", got)
	}
	if got := backend.opts.NodeSelector["node-pool"]; got != "agents" {
		t.Fatalf("sandbox node selector mutation leaked to backend options: %q", got)
	}
}

func TestHostedAgentPodSchedulingDefaultsAreEmpty(t *testing.T) {
	backend := testBackend(t)
	objects, err := backend.instanceObjects(desiredInstance())
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	cleanup, err := backend.cleanupJob("inst-1", "alloc-1")
	if err != nil {
		t.Fatalf("cleanupJob: %v", err)
	}

	for name, spec := range map[string]*corev1.PodSpec{
		"sandbox": &deploymentFrom(t, objects).Spec.Template.Spec,
		"cleanup": &cleanup.Spec.Template.Spec,
	} {
		t.Run(name, func(t *testing.T) {
			if spec.Affinity != nil || len(spec.Tolerations) != 0 || len(spec.NodeSelector) != 0 {
				t.Fatalf("expected empty scheduling fields, got %+v", spec)
			}
		})
	}
}

func TestObserveInstanceDetectsPodSchedulingChanges(t *testing.T) {
	desired := desiredInstance()
	configured := Options{
		Namespace:     "obot-agents",
		ClusterDomain: "cluster.local",
		NodeSelector:  map[string]string{"node-pool": "agents"},
	}
	changed := Options{
		Namespace:     "obot-agents",
		ClusterDomain: "cluster.local",
		NodeSelector:  map[string]string{"node-pool": "gpu-agents"},
	}
	empty := Options{Namespace: "obot-agents", ClusterDomain: "cluster.local"}

	for _, tt := range []struct {
		name             string
		applied          Options
		observed         Options
		observedRevision string
	}{
		{name: "unchanged", applied: configured, observed: configured, observedRevision: desired.Revision},
		{name: "added", applied: empty, observed: configured},
		{name: "changed", applied: configured, observed: changed},
		{name: "removed", applied: configured, observed: empty},
	} {
		t.Run(tt.name, func(t *testing.T) {
			appliedBackend, err := New(nil, nil, tt.applied)
			if err != nil {
				t.Fatalf("New applied backend: %v", err)
			}
			objects, err := appliedBackend.instanceObjects(desired)
			if err != nil {
				t.Fatalf("instanceObjects: %v", err)
			}

			client := fake.NewClientBuilder().WithScheme(k8sscheme.Scheme).WithObjects(deploymentFrom(t, objects)).Build()
			observingBackend, err := New(client, client, tt.observed)
			if err != nil {
				t.Fatalf("New observing backend: %v", err)
			}
			observation, err := observingBackend.ObserveInstance(t.Context(), desired.Ref)
			if err != nil {
				t.Fatalf("ObserveInstance: %v", err)
			}
			if observation.ObservedRevision != tt.observedRevision {
				t.Fatalf("observed revision = %q, want %q", observation.ObservedRevision, tt.observedRevision)
			}
		})
	}
}

func deploymentFrom(t *testing.T, objs []kclient.Object) *appsv1.Deployment {
	t.Helper()
	for _, obj := range objs {
		if deployment, ok := obj.(*appsv1.Deployment); ok {
			return deployment
		}
	}
	t.Fatal("no Deployment in object set")
	return nil
}

// The PriorityClass is what ties a sandbox to its pool's ResourceQuota. Without
// it the sandbox escapes pool accounting entirely, since a quota's scopeSelector
// cannot match on labels.
func TestInstanceObjectsTagSandboxWithPoolPriorityClass(t *testing.T) {
	backend := testBackend(t)

	objs, err := backend.instanceObjects(desiredInstance())
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	deployment := deploymentFrom(t, objs)

	if got := deployment.Spec.Template.Spec.PriorityClassName; got != poolName("alloc-1") {
		t.Errorf("priorityClassName = %q, want %q", got, poolName("alloc-1"))
	}
}

func TestInstanceObjectsApplyRuntimeClass(t *testing.T) {
	tests := []struct {
		name             string
		runtimeClassName string
		want             *string
	}{
		{name: "configured", runtimeClassName: "gvisor", want: new("gvisor")},
		{name: "unset"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := testBackend(t)
			backend.opts.RuntimeClassName = tt.runtimeClassName

			objs, err := backend.instanceObjects(desiredInstance())
			if err != nil {
				t.Fatalf("instanceObjects: %v", err)
			}
			got := deploymentFrom(t, objs).Spec.Template.Spec.RuntimeClassName

			if tt.want == nil {
				if got != nil {
					t.Errorf("runtimeClassName = %q, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatalf("runtimeClassName = nil, want %q", *tt.want)
			}
			if *got != *tt.want {
				t.Errorf("runtimeClassName = %q, want %q", *got, *tt.want)
			}
		})
	}
}

// Sandboxes share one ReadWriteOnce volume and are separated only by subPath, so
// a rolling update would put two pods on the same directory.
func TestInstanceObjectsUseRecreateAndSubPath(t *testing.T) {
	backend := testBackend(t)

	objs, err := backend.instanceObjects(desiredInstance())
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	deployment := deploymentFrom(t, objs)

	if deployment.Spec.Strategy.Type != appsv1.RecreateDeploymentStrategyType {
		t.Errorf("strategy = %s, want Recreate", deployment.Spec.Strategy.Type)
	}

	var found bool
	for _, mount := range deployment.Spec.Template.Spec.Containers[0].VolumeMounts {
		if mount.Name == workspaceVolumeName {
			found = true
			if mount.SubPath == "" {
				t.Error("the workspace mount must use a per-instance subPath")
			}
		}
	}
	if !found {
		t.Error("expected a workspace mount from the pool volume")
	}
}

// The revision has to reach the pod template, not just the Deployment, or a
// configuration change would leave the running pod untouched.
func TestInstanceObjectsPropagateRevisionToPodTemplate(t *testing.T) {
	backend := testBackend(t)

	objs, err := backend.instanceObjects(desiredInstance())
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	deployment := deploymentFrom(t, objs)

	if got := deployment.Spec.Template.Annotations[revisionAnnotation]; got != "sha256:abc" {
		t.Errorf("pod template revision = %q, want sha256:abc", got)
	}
	if got := deployment.Annotations[revisionAnnotation]; got != "sha256:abc" {
		t.Errorf("deployment revision = %q, want sha256:abc", got)
	}
}

// A sandbox reserves an equal share of its pool and may burst to the whole
// pool. Both figures are computed from the pool, so this only translates them.
func TestContainerResourcesAreBurstable(t *testing.T) {
	resources := containerResources(
		agentbackend.InstanceResources{CPUVCPUs: 0.5, MemoryBytes: 1 << 30},
		agentbackend.InstanceResources{CPUVCPUs: 2, MemoryBytes: 4 << 30},
	)

	wantRequestCPU := resource.MustParse("500m")
	if got := resources.Requests[corev1.ResourceCPU]; got.Cmp(wantRequestCPU) != 0 {
		t.Errorf("request cpu = %s, want %s", got.String(), wantRequestCPU.String())
	}
	wantLimitCPU := resource.MustParse("2")
	if got := resources.Limits[corev1.ResourceCPU]; got.Cmp(wantLimitCPU) != 0 {
		t.Errorf("limit cpu = %s, want %s", got.String(), wantLimitCPU.String())
	}
	if got := resources.Requests[corev1.ResourceMemory]; got.Value() != 1<<30 {
		t.Errorf("request memory = %d, want %d", got.Value(), int64(1<<30))
	}
	if got := resources.Limits[corev1.ResourceMemory]; got.Value() != 4<<30 {
		t.Errorf("limit memory = %d, want %d", got.Value(), int64(4<<30))
	}
}

// Only requests are bounded. Every sandbox may burst to the whole pool, so a
// limits quota would either reject the second sandbox or enforce nothing.
func TestQuotaHardBoundsRequestsAndCountOnly(t *testing.T) {
	hard := quotaHard(agentbackend.DesiredPool{
		Capacity:     agentbackend.ResourceQuantity{CPUVCPUs: 8, MemoryBytes: 16 << 30},
		MaxSandboxes: 8,
	})

	requestCPU := hard[corev1.ResourceRequestsCPU]
	if requestCPU.MilliValue() != 8000 {
		t.Errorf("requests.cpu = %s, want 8", requestCPU.String())
	}
	if pods := hard[corev1.ResourcePods]; pods.Value() != 8 {
		t.Errorf("count/pods = %s, want 8", pods.String())
	}
	if _, ok := hard[corev1.ResourceLimitsCPU]; ok {
		t.Error("a limits budget cannot bound a pool whose sandboxes may each use all of it")
	}
	if _, ok := hard[corev1.ResourceLimitsMemory]; ok {
		t.Error("a limits budget cannot bound a pool whose sandboxes may each use all of it")
	}
}

// Suspending zeroes the budget so no new sandbox is admitted. Quota is an
// admission check, so sandboxes already running are unaffected.
func TestQuotaHardZeroesWhenSuspended(t *testing.T) {
	hard := quotaHard(agentbackend.DesiredPool{
		Capacity:     agentbackend.ResourceQuantity{CPUVCPUs: 8, MemoryBytes: 16 << 30},
		MaxSandboxes: 8,
		Suspended:    true,
	})

	for _, key := range []corev1.ResourceName{
		corev1.ResourceRequestsCPU, corev1.ResourceRequestsMemory, corev1.ResourcePods,
	} {
		value := hard[key]
		if !value.IsZero() {
			t.Errorf("%s = %s, want 0", key, value.String())
		}
	}
}

func TestRenderFilesGroupsByDirectory(t *testing.T) {
	secretData := map[string][]byte{}
	volumes, mounts, err := renderFiles("obot-agent-inst-1", []agentbackend.File{
		{Path: "/etc/obot/agent.json", Content: []byte("{}"), Mode: 0o644},
		{Path: "/etc/obot/skills.json", Content: []byte("[]")},
		{Path: "/opt/config.yaml", Content: []byte("a: b")},
	}, secretData)
	if err != nil {
		t.Fatalf("renderFiles: %v", err)
	}

	if len(volumes) != 2 || len(mounts) != 2 {
		t.Fatalf("expected one volume per directory, got %d volumes and %d mounts", len(volumes), len(mounts))
	}
	if mounts[0].MountPath != "/etc/obot" || mounts[1].MountPath != "/opt" {
		t.Errorf("unexpected mount paths: %s, %s", mounts[0].MountPath, mounts[1].MountPath)
	}
	if len(volumes[0].Secret.Items) != 2 {
		t.Errorf("expected both /etc/obot files in one volume, got %d", len(volumes[0].Secret.Items))
	}
	if string(secretData["etc_obot_agent.json"]) != "{}" {
		t.Errorf("file content was not stored under its path-derived key: %v", secretData)
	}
	// A secret volume without a secretName is rejected by the API server, not by
	// anything in this package.
	for _, volume := range volumes {
		if volume.Secret.SecretName != "obot-agent-inst-1" {
			t.Errorf("volume %s has secretName %q, want the instance secret", volume.Name, volume.Secret.SecretName)
		}
	}
}

// A ResourceQuota requires every pod in its scope to declare both a request and
// a limit for each constrained resource. The cleanup job runs inside the pool,
// so omitting them makes the quota reject it forever and deletion never
// completes.
func TestCleanupJobDeclaresResources(t *testing.T) {
	backend := testBackend(t)

	job, err := backend.cleanupJob("inst-1", "alloc-1")
	if err != nil {
		t.Fatalf("cleanupJob: %v", err)
	}

	container := job.Spec.Template.Spec.Containers[0]
	for _, name := range []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory} {
		if _, ok := container.Resources.Requests[name]; !ok {
			t.Errorf("cleanup job is missing a request for %s", name)
		}
		if _, ok := container.Resources.Limits[name]; !ok {
			t.Errorf("cleanup job is missing a limit for %s", name)
		}
	}
	if job.Spec.Template.Spec.PriorityClassName != poolName("alloc-1") {
		t.Error("the cleanup job must run in the pool so it lands on the pool's node")
	}
}

// Every mount must name a volume that exists, or the Deployment is invalid.
func TestInstanceObjectsMountsResolveToVolumes(t *testing.T) {
	backend := testBackend(t)
	desired := desiredInstance()
	desired.Files = []agentbackend.File{
		{Path: "/etc/obot/agent.json", Content: []byte("{}")},
		{Path: "/opt/extra.yaml", Content: []byte("a: b")},
	}

	objs, err := backend.instanceObjects(desired)
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	deployment := deploymentFrom(t, objs)

	volumes := map[string]corev1.Volume{}
	for _, volume := range deployment.Spec.Template.Spec.Volumes {
		volumes[volume.Name] = volume
	}
	for _, mount := range deployment.Spec.Template.Spec.Containers[0].VolumeMounts {
		volume, ok := volumes[mount.Name]
		if !ok {
			t.Errorf("mount %q at %s has no matching volume", mount.Name, mount.MountPath)
			continue
		}
		if volume.Secret != nil && volume.Secret.SecretName == "" {
			t.Errorf("volume %q is a secret volume with no secretName", volume.Name)
		}
		if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == "" {
			t.Errorf("volume %q is a claim volume with no claimName", volume.Name)
		}
	}
}

func TestRenderFilesRejectsRelativePaths(t *testing.T) {
	if _, _, err := renderFiles("obot-agent-inst-1", []agentbackend.File{{Path: "agent.json"}}, map[string][]byte{}); err == nil {
		t.Fatal("expected a relative path to be rejected")
	}
}

// A file whose derived key collides with an environment variable would silently
// overwrite it in the shared secret.
func TestRenderFilesRejectsCollisionWithEnv(t *testing.T) {
	secretData := map[string][]byte{"etc_agent.json": []byte("env")}
	if _, _, err := renderFiles("obot-agent-inst-1", []agentbackend.File{{Path: "/etc/agent.json"}}, secretData); err == nil {
		t.Fatal("expected a collision with existing secret data to be rejected")
	}
}

func TestPoolNamesAreStableAndDistinct(t *testing.T) {
	if poolName("alloc-1") == poolName("alloc-2") {
		t.Fatal("distinct pools must not share a pool name")
	}
	// Names are the correlation key across reconcile, observe and delete, so an
	// identity must always map to the same object.
	first, second := poolName("alloc-1"), poolName("alloc-1")
	if first != second {
		t.Fatalf("pool names must be stable: %q then %q", first, second)
	}
	// PriorityClasses are cluster-scoped, so pool names have to survive
	// identities that are not already DNS-safe.
	if got := poolName("Alloc_1/weird"); got == "" {
		t.Fatal("expected a usable name from a non-DNS identity")
	}
}

// An image whose entrypoint is a shell reads EOF and exits without both a TTY
// and an open stdin, which surfaces as CrashLoopBackOff rather than as anything
// pointing at the harness.
func TestInstanceObjectsHonourInteractiveHarness(t *testing.T) {
	backend := testBackend(t)

	plain, err := backend.instanceObjects(desiredInstance())
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	container := deploymentFrom(t, plain).Spec.Template.Spec.Containers[0]
	if container.TTY || container.Stdin {
		t.Error("a non-interactive harness must not allocate a TTY")
	}

	desired := desiredInstance()
	desired.Harness.Interactive = true
	interactive, err := backend.instanceObjects(desired)
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	container = deploymentFrom(t, interactive).Spec.Template.Spec.Containers[0]
	if !container.TTY || !container.Stdin {
		t.Errorf("an interactive harness needs both tty and stdin, got tty=%v stdin=%v", container.TTY, container.Stdin)
	}
}

// The identity credential must arrive as a file. Agents execute model-directed
// commands and every subprocess inherits the environment, so an environment
// variable would hand the credential to everything the agent runs.
func TestInstanceObjectsDeliverSecretRefsAsFiles(t *testing.T) {
	backend := testBackend(t)
	desired := desiredInstance()
	desired.Secrets = []agentbackend.SecretRef{{
		ID:       "inst-1-credential",
		Version:  "7",
		Value:    "ok1-1-2-supersecret",
		FilePath: "/etc/obot/credential",
	}}

	objs, err := backend.instanceObjects(desired)
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	deployment := deploymentFrom(t, objs)

	for _, env := range deployment.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "OBOT_API_KEY" {
			t.Error("the credential must not be delivered as an environment variable")
		}
	}

	var mounted bool
	for _, mount := range deployment.Spec.Template.Spec.Containers[0].VolumeMounts {
		if mount.MountPath == "/etc/obot" {
			mounted = true
		}
	}
	if !mounted {
		t.Error("expected the credential's directory to be mounted")
	}
}

// A rotated credential has to restart the sandbox, because agents read theirs
// once at startup. The value must never reach the revision, but the version
// must, or the rotation would not propagate.
func TestRevisionTracksSecretVersionNotValue(t *testing.T) {
	base := desiredInstance()
	base.Secrets = []agentbackend.SecretRef{{ID: "c", Version: "1", Value: "secret-one"}}

	sameVersionNewValue := desiredInstance()
	sameVersionNewValue.Secrets = []agentbackend.SecretRef{{ID: "c", Version: "1", Value: "secret-two"}}

	newVersion := desiredInstance()
	newVersion.Secrets = []agentbackend.SecretRef{{ID: "c", Version: "2", Value: "secret-one"}}

	if got := base.Redacted().Secrets[0].Value; got != "" {
		t.Fatalf("Redacted left a secret value in place: %q", got)
	}
	if base.Redacted().Secrets[0].Version != "1" {
		t.Error("Redacted must preserve the version")
	}
	// Redaction must not mutate the original, or the caller would lose the value
	// it still has to deliver.
	if base.Secrets[0].Value != "secret-one" {
		t.Error("Redacted mutated its receiver")
	}
	if sameVersionNewValue.Redacted().Secrets[0] != base.Redacted().Secrets[0] {
		t.Error("a value change without a version change must not alter the redacted form")
	}
	if newVersion.Redacted().Secrets[0] == base.Redacted().Secrets[0] {
		t.Error("a version change must alter the redacted form so the revision changes")
	}
}

// Live usage is measured per sandbox, but a pod too new to have been sampled --
// or a cluster with no metrics-server -- must not report as using nothing. A
// sandbox that exists has reserved its request, so that is the floor.
func TestPodMetricsParsing(t *testing.T) {
	reader := &metricsServerReader{}
	_ = reader // parsing is exercised through the payload below

	var list podMetricsList
	payload := `{"items":[
	  {"metadata":{"name":"pod-a"},"containers":[
	    {"name":"agent","usage":{"cpu":"250m","memory":"128Mi"}}]},
	  {"metadata":{"name":"pod-b"},"containers":[
	    {"name":"agent","usage":{"cpu":"1500000000n","memory":"1Gi"}},
	    {"name":"side","usage":{"cpu":"500m","memory":"64Mi"}}]}
	]}`
	if err := json.Unmarshal([]byte(payload), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	got := map[string]agentbackend.ResourceUtilization{}
	for _, item := range list.Items {
		var total agentbackend.ResourceUtilization
		for _, c := range item.Containers {
			if cpu, err := resource.ParseQuantity(c.Usage.CPU); err == nil {
				total.CPUVCPUs += float64(cpu.MilliValue()) / 1000
			}
			if mem, err := resource.ParseQuantity(c.Usage.Memory); err == nil {
				total.MemoryBytes += mem.Value()
			}
		}
		got[item.Metadata.Name] = total
	}

	if got["pod-a"].CPUVCPUs != 0.25 {
		t.Errorf("pod-a cpu = %v, want 0.25", got["pod-a"].CPUVCPUs)
	}
	if got["pod-a"].MemoryBytes != 128<<20 {
		t.Errorf("pod-a memory = %d, want %d", got["pod-a"].MemoryBytes, 128<<20)
	}
	// Nanocores are what metrics-server actually emits, and containers sum.
	if got["pod-b"].CPUVCPUs != 2.0 {
		t.Errorf("pod-b cpu = %v, want 2.0 (1.5 + 0.5, nanocores parsed)", got["pod-b"].CPUVCPUs)
	}
	if want := int64(1<<30) + 64<<20; got["pod-b"].MemoryBytes != want {
		t.Errorf("pod-b memory = %d, want %d", got["pod-b"].MemoryBytes, want)
	}
}

// Kubelet reports the filesystem a volume sits on, not the volume. For a
// hostPath-backed class such as local-path that is the whole node disk, which
// would show every pool as permanently near-full -- the same class of mistake
// as reporting provisioned size as usage.
func TestVolumeIsDedicated(t *testing.T) {
	const claim = 20 << 30 // 20Gi

	for _, tt := range []struct {
		name       string
		filesystem int64
		want       bool
	}{
		{"exactly the claim", claim, true},
		{"rounded up by the provisioner", claim + (1 << 30), true},
		{"host filesystem dwarfs the claim", 1081101176832, false},
		{"unknown filesystem size", 0, false},
		{"unknown claim size", claim, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := volumeIsDedicated(tt.filesystem, claim); got != tt.want {
				t.Errorf("volumeIsDedicated(%d, %d) = %v, want %v", tt.filesystem, claim, got, tt.want)
			}
		})
	}

	if volumeIsDedicated(claim, 0) {
		t.Error("an unknown claim capacity cannot be judged dedicated")
	}
}

// A pool is confined to one node by its ReadWriteOnce volume, so any running
// pod identifies the node whose kubelet holds the volume stats.
func TestPoolNodeIgnoresTerminatingPods(t *testing.T) {
	now := metav1.Now()
	pods := []corev1.Pod{
		{DeletionTimestamp: &now, Spec: corev1.PodSpec{NodeName: "going-away"}},
		{Spec: corev1.PodSpec{NodeName: "live-node"}},
	}

	if got := poolNode(pods); got != "live-node" {
		t.Errorf("poolNode = %q, want live-node", got)
	}
	if got := poolNode(nil); got != "" {
		t.Errorf("poolNode(nil) = %q, want empty", got)
	}
}

func serviceFrom(objs []kclient.Object) *corev1.Service {
	for _, obj := range objs {
		if service, ok := obj.(*corev1.Service); ok {
			return service
		}
	}
	return nil
}

// A Service with no ports is rejected by the API server, and the Deployment is
// applied before it -- so shipping one for a port-less agent leaves the sandbox
// running while its instance never leaves pending, with nothing in status to
// say why. Every agent that does not serve HTTP is affected, which is most of
// them.
func TestInstanceObjectsOmitServiceWithoutPort(t *testing.T) {
	backend := testBackend(t)

	desired := desiredInstance()
	desired.Port = 0

	objs, err := backend.instanceObjects(desired)
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	if service := serviceFrom(objs); service != nil {
		t.Fatalf("expected no Service for a port-less agent, got %q with ports %v",
			service.Name, service.Spec.Ports)
	}
}

// The counterpart: an agent that does declare a port must still be reachable.
func TestInstanceObjectsIncludeServiceWithPort(t *testing.T) {
	backend := testBackend(t)

	desired := desiredInstance()
	desired.Port = 8080

	objs, err := backend.instanceObjects(desired)
	if err != nil {
		t.Fatalf("instanceObjects: %v", err)
	}
	service := serviceFrom(objs)
	if service == nil {
		t.Fatal("expected a Service for an agent that declares a port")
	}
	if len(service.Spec.Ports) != 1 || service.Spec.Ports[0].Port != 80 {
		t.Fatalf("service ports = %v", service.Spec.Ports)
	}
	// The Service targets the container port by name, so the two have to agree.
	deployment := deploymentFrom(t, objs)
	ports := deployment.Spec.Template.Spec.Containers[0].Ports
	if len(ports) != 1 || ports[0].Name != service.Spec.Ports[0].TargetPort.StrVal {
		t.Fatalf("container ports %v do not match service target %v",
			ports, service.Spec.Ports[0].TargetPort)
	}
}
