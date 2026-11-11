package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeCapacity reports the same capacity for every server.
type fakeCapacity types.MCPCapacityInfo

// testEntitlements grants the same entitlements every time.
type testEntitlements types.Entitlements

func testComponent(id string, runtime types.Runtime, singleUser bool, requests types.MCPResourceRequests) types.VMCPComponent {
	component := types.VMCPComponent{
		ID:              id,
		Name:            id,
		ForceSingleUser: singleUser,
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: runtime,
			},
		},
	}
	if requests != (types.MCPResourceRequests{}) {
		component.CatalogEntry.Manifest.Resources = &types.MCPResourceRequirements{
			Requests: requests,
		}
	}
	return component
}

// testVMCP returns a vMCP whose profile enables every component for everyone.
func testVMCP(name string, components ...types.VMCPComponent) *v1.VMCP {
	return &v1.VMCP{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: components,
				Profiles: []types.VMCPProfile{{
					Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
					Permissions: types.VMCPProfilePermissions{AllowAllComponents: true},
				}},
			},
		},
	}
}

func testInstance(name, vmcpID string) *v1.VMCPInstance {
	return &v1.VMCPInstance{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			UserID: "7",
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcpID,
			},
		},
	}
}

func testFilter(name string, manifest types.MCPWebhookValidationManifest) *v1.MCPWebhookValidation {
	return &v1.MCPWebhookValidation{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPWebhookValidationSpec{
			Manifest: manifest,
		},
	}
}

func testSystemServer(name, filter string, manifest types.SystemMCPServerManifest) *v1.SystemMCPServer {
	return &v1.SystemMCPServer{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.SystemMCPServerSpec{
			WebhookValidationName: filter,
			Manifest:              manifest,
		},
	}
}

func deleted[T interface{ SetDeletionTimestamp(*metav1.Time) }](obj T) T {
	now := metav1.Now()
	obj.SetDeletionTimestamp(&now)
	return obj
}

func TestComputeUsage(t *testing.T) {
	shared := testComponent("shared", types.RuntimeNPX, false, types.MCPResourceRequests{})
	dedicated := testComponent("dedicated", types.RuntimeUVX, true, types.MCPResourceRequests{})
	remote := testComponent("remote", types.RuntimeRemote, true, types.MCPResourceRequests{})
	sized := testComponent("sized", types.RuntimeContainerized, true, types.MCPResourceRequests{
		CPU:    "500m",
		Memory: "1Gi",
	})
	webhook := types.MCPWebhookValidationManifest{
		URL: "https://filter.example.com",
	}
	pii := types.MCPWebhookValidationManifest{
		SystemMCPServerManifest: &types.SystemMCPServerManifest{
			Enabled: new(true),
			Runtime: types.RuntimeContainerized,
			Resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{
					CPU:    "250m",
					Memory: "512Mi",
				},
			},
		},
	}

	for _, tt := range []struct {
		name string
		objs []kclient.Object
		want types.EntitlementsUsage
	}{
		{
			name: "a multi-user component runs once for its vMCP",
			objs: []kclient.Object{
				testVMCP("vmcp1", shared),
				testInstance("instance1", "vmcp1"),
				testInstance("instance2", "vmcp1"),
			},
			want: types.EntitlementsUsage{
				MemoryMiB:     200,
				CPUMillicores: 10,
			},
		},
		{
			name: "a single-user component runs once for each instance",
			objs: []kclient.Object{
				testVMCP("vmcp1", dedicated),
				testInstance("instance1", "vmcp1"),
				testInstance("instance2", "vmcp1"),
			},
			want: types.EntitlementsUsage{
				MemoryMiB:     400,
				CPUMillicores: 20,
			},
		},
		{
			name: "remote components don't run in a pod",
			objs: []kclient.Object{
				testVMCP("vmcp1", remote),
				testInstance("instance1", "vmcp1"),
			},
		},
		{
			name: "a component's own requests replace the defaults",
			objs: []kclient.Object{
				testVMCP("vmcp1", sized),
				testInstance("instance1", "vmcp1"),
			},
			want: types.EntitlementsUsage{
				MemoryMiB:     1024,
				CPUMillicores: 500,
			},
		},
		{
			name: "the K8s settings set the default requests",
			objs: []kclient.Object{
				&v1.K8sSettings{
					Name:      system.K8sSettingsName,
					Namespace: system.DefaultNamespace,
					Spec: v1.K8sSettingsSpec{
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
								corev1.ResourceCPU:    resource.MustParse("100m"),
							},
						},
					},
				},
				testVMCP("vmcp1", shared),
			},
			want: types.EntitlementsUsage{
				MemoryMiB:     512,
				CPUMillicores: 100,
			},
		},
		{
			name: "a memory limit without a request counts as the request",
			objs: []kclient.Object{
				&v1.K8sSettings{
					Name:      system.K8sSettingsName,
					Namespace: system.DefaultNamespace,
					Spec: v1.K8sSettingsSpec{
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
						},
					},
				},
				testVMCP("vmcp1", shared),
			},
			want: types.EntitlementsUsage{
				MemoryMiB:     1024,
				CPUMillicores: 10,
			},
		},
		{
			name: "deleted vMCPs and instances don't count",
			objs: []kclient.Object{
				deleted(testVMCP("vmcp1", shared, dedicated)),
				testVMCP("vmcp2", dedicated),
				testInstance("instance1", "vmcp1"),
				deleted(testInstance("instance2", "vmcp2")),
			},
		},
		{
			name: "instances of a missing vMCP don't count",
			objs: []kclient.Object{
				testInstance("instance1", "missing"),
			},
		},
		{
			name: "a webhook filter runs a server with the default requests",
			objs: []kclient.Object{
				testFilter("filter1", webhook),
			},
			want: types.EntitlementsUsage{
				MemoryMiB:     200,
				CPUMillicores: 10,
			},
		},
		{
			name: "a filter's server manifest sets its requests",
			objs: []kclient.Object{
				testFilter("filter1", pii),
			},
			want: types.EntitlementsUsage{
				MemoryMiB:     512,
				CPUMillicores: 250,
			},
		},
		{
			name: "a filter's server counts once",
			objs: []kclient.Object{
				testFilter("filter1", pii),
				testSystemServer("sms1", "filter1", *pii.SystemMCPServerManifest),
			},
			want: types.EntitlementsUsage{
				MemoryMiB:     512,
				CPUMillicores: 250,
			},
		},
		{
			name: "disabled and guardrail filters don't run a server",
			objs: []kclient.Object{
				testFilter("filter1", types.MCPWebhookValidationManifest{
					URL:      "https://filter.example.com",
					Disabled: true,
				}),
				testFilter("filter2", types.MCPWebhookValidationManifest{
					SystemMCPServerManifest: &types.SystemMCPServerManifest{
						Enabled: new(false),
						Runtime: types.RuntimeContainerized,
					},
				}),
				testFilter("filter3", types.MCPWebhookValidationManifest{
					Guardrail: &types.MCPGuardrailFilterConfig{},
				}),
			},
		},
		{
			name: "a system server runs unless it's disabled or remote",
			objs: []kclient.Object{
				testSystemServer("sms1", "", types.SystemMCPServerManifest{
					Runtime: types.RuntimeNPX,
				}),
				testSystemServer("sms2", "", types.SystemMCPServerManifest{
					Enabled: new(false),
					Runtime: types.RuntimeNPX,
				}),
				testSystemServer("sms3", "", types.SystemMCPServerManifest{
					Runtime: types.RuntimeRemote,
				}),
			},
			want: types.EntitlementsUsage{
				MemoryMiB:     200,
				CPUMillicores: 10,
			},
		},
		{
			name: "deleted filters and system servers don't count",
			objs: []kclient.Object{
				deleted(testFilter("filter1", webhook)),
				deleted(testSystemServer("sms1", "", types.SystemMCPServerManifest{
					Runtime: types.RuntimeNPX,
				})),
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var compute Compute
			for _, obj := range tt.objs {
				compute.replace(obj)
			}
			got, err := compute.Usage()
			if err != nil {
				t.Fatalf("Usage() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Usage() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// The usage counted for a server has to be what its Deployment requests, since the count is what's admitted and the
// Deployment is what runs.
func TestComputeUsageMatchesTheDeployment(t *testing.T) {
	settings := func(resources corev1.ResourceRequirements) []v1.K8sSettings {
		return []v1.K8sSettings{{
			Name:      system.K8sSettingsName,
			Namespace: system.DefaultNamespace,
			Spec: v1.K8sSettingsSpec{
				Resources: &resources,
			},
		}}
	}

	for _, tt := range []struct {
		name      string
		runtime   types.Runtime
		resources *types.MCPResourceRequirements
		settings  []v1.K8sSettings
	}{
		{
			name:    "the built-in defaults",
			runtime: types.RuntimeContainerized,
		},
		{
			name:    "a command-based server",
			runtime: types.RuntimeNPX,
		},
		{
			name:    "an OpenAPI server",
			runtime: types.RuntimeOpenAPI,
		},
		{
			name:    "the server's own requests",
			runtime: types.RuntimeContainerized,
			resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{
					CPU:    "500m",
					Memory: "1Gi",
				},
			},
		},
		{
			name:    "a memory request that isn't a whole MiB rounds up",
			runtime: types.RuntimeContainerized,
			resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{
					Memory: "100M",
				},
			},
		},
		{
			name:    "the server's limits without requests",
			runtime: types.RuntimeContainerized,
			resources: &types.MCPResourceRequirements{
				Limits: types.MCPResourceRequests{
					CPU:    "2",
					Memory: "2Gi",
				},
			},
		},
		{
			name:    "the K8s settings' requests",
			runtime: types.RuntimeContainerized,
			settings: settings(corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("512Mi"),
					corev1.ResourceCPU:    resource.MustParse("100m"),
				},
			}),
		},
		{
			name:    "the K8s settings' limits without requests",
			runtime: types.RuntimeContainerized,
			settings: settings(corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			}),
		},
		{
			name:    "the server's requests over the K8s settings'",
			runtime: types.RuntimeContainerized,
			resources: &types.MCPResourceRequirements{
				Requests: types.MCPResourceRequests{
					Memory: "256Mi",
				},
			},
			settings: settings(corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("512Mi"),
					corev1.ResourceCPU:    resource.MustParse("100m"),
				},
			}),
		},
		{
			name: "K8s settings under another name aren't the ones servers deploy with",
			settings: []v1.K8sSettings{{
				Name:      "other",
				Namespace: system.DefaultNamespace,
				Spec: v1.K8sSettingsSpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			}},
			runtime: types.RuntimeContainerized,
		},
		{
			name:    "a remote server has no Deployment",
			runtime: types.RuntimeRemote,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// What the backend deploys for the server.
			stored := make([]kclient.Object, 0, len(tt.settings))
			for i := range tt.settings {
				stored = append(stored, &tt.settings[i])
			}
			resources, err := CoreResourceRequirements(tt.resources)
			if err != nil {
				t.Fatal(err)
			}
			server := testK8sServerConfig()
			server.Runtime, server.Resources = tt.runtime, resources
			objs, err := newTestKubernetesBackend(t, stored...).k8sObjects(t.Context(), server)
			if err != nil {
				t.Fatalf("k8sObjects() error = %v", err)
			}
			var deployed types.EntitlementsUsage
			for _, obj := range objs {
				deployment, ok := obj.(*appsv1.Deployment)
				if !ok {
					continue
				}
				if deployment.Spec.Replicas != nil && *deployment.Spec.Replicas != 1 {
					t.Fatalf("the Deployment runs %d replicas, and usage counts one", *deployment.Spec.Replicas)
				}
				pod := deployment.Spec.Template.Spec
				for _, container := range append(pod.InitContainers, pod.Containers...) {
					memory, cpu := requested(container.Resources, corev1.ResourceMemory), requested(container.Resources, corev1.ResourceCPU)
					deployed.MemoryMiB += (memory.Value() + mebibyte - 1) / mebibyte
					deployed.CPUMillicores += cpu.MilliValue()
				}
			}

			// What's counted for the vMCP component that runs it.
			component := testComponent("component", tt.runtime, false, types.MCPResourceRequests{})
			component.CatalogEntry.Manifest.Resources = tt.resources
			counted, err := Compute{
				VMCPs:    []v1.VMCP{*testVMCP("vmcp1", component)},
				Settings: tt.settings,
			}.Usage()
			if err != nil {
				t.Fatalf("Usage() error = %v", err)
			}

			if counted != deployed {
				t.Fatalf("Usage() = %+v, but the Deployment requests %+v", counted, deployed)
			}
			if tt.runtime != types.RuntimeRemote && counted == (types.EntitlementsUsage{}) {
				t.Fatal("a hosted server counted for nothing")
			}
		})
	}
}

func (e testEntitlements) Entitlements(context.Context) (types.Entitlements, error) {
	return types.Entitlements(e), nil
}

func TestComputeAdmitterAdmit(t *testing.T) {
	// Each instance runs the dedicated component with the default 200 MiB request.
	dedicated := types.VMCPComponent{
		ID:              "dedicated",
		Name:            "dedicated",
		ForceSingleUser: true,
		CatalogEntry: types.MCPServerCatalogEntrySnapshot{
			Manifest: types.MCPServerCatalogEntryManifest{
				Runtime: types.RuntimeNPX,
			},
		},
	}
	memory := func(miB int64) types.Entitlements {
		return types.Entitlements{MemoryMiB: new(miB)}
	}

	for _, tt := range []struct {
		name        string
		granted     types.Entitlements
		obj         kclient.Object
		update      bool
		wantLock    bool
		wantRefused bool
	}{
		{
			name:     "a write within the limits is made under the lock",
			granted:  memory(400),
			obj:      testInstance("instance2", "vmcp1"),
			wantLock: true,
		},
		{
			name:        "a write past the limits is refused",
			granted:     memory(200),
			obj:         testInstance("instance2", "vmcp1"),
			wantLock:    true,
			wantRefused: true,
		},
		{
			name:     "a write that lowers usage is made while over",
			granted:  memory(0),
			obj:      testVMCP("vmcp1"),
			update:   true,
			wantLock: true,
		},
		{
			name:    "a filter past the limits is refused",
			granted: memory(200),
			obj: &v1.MCPWebhookValidation{
				Namespace: system.DefaultNamespace,
				Name:      "filter1",
				Spec: v1.MCPWebhookValidationSpec{
					Manifest: types.MCPWebhookValidationManifest{
						URL: "https://filter.example.com",
					},
				},
			},
			wantLock:    true,
			wantRefused: true,
		},
		{
			name:    "a system server past the limits is refused",
			granted: memory(200),
			obj: &v1.SystemMCPServer{
				Namespace: system.DefaultNamespace,
				Name:      "sms1",
				Spec: v1.SystemMCPServerSpec{
					Manifest: types.SystemMCPServerManifest{
						Runtime: types.RuntimeNPX,
					},
				},
			},
			wantLock:    true,
			wantRefused: true,
		},
		{
			name:    "a filter's own server is made at the limit",
			granted: memory(200),
			obj: &v1.SystemMCPServer{
				Namespace: system.DefaultNamespace,
				Name:      "sms1",
				Spec: v1.SystemMCPServerSpec{
					WebhookValidationName: "filter1",
					Manifest: types.SystemMCPServerManifest{
						Runtime: types.RuntimeNPX,
					},
				},
			},
			wantLock: true,
		},
		{
			name:     "without limits nothing is refused",
			obj:      testInstance("instance2", "vmcp1"),
			wantLock: true,
		},
		{
			name:    "other kinds skip the lock",
			granted: memory(0),
			obj: &v1.MCPCatalog{
				Namespace: system.DefaultNamespace,
				Name:      "catalog",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(testVMCP("vmcp1", dedicated), testInstance("instance1", "vmcp1")).Build()
			var locked int

			write := func() error {
				return storage.Create(t.Context(), tt.obj)
			}
			if tt.update {
				stored := tt.obj.DeepCopyObject().(kclient.Object)
				if err := storage.Get(t.Context(), kclient.ObjectKeyFromObject(tt.obj), stored); err != nil {
					t.Fatal(err)
				}
				tt.obj.SetResourceVersion(stored.GetResourceVersion())
				write = func() error {
					return storage.Update(t.Context(), tt.obj)
				}
			}

			withLock := func(_ context.Context, fn func() error) error {
				locked++
				return fn()
			}
			err := NewComputeAdmitter(storage, testEntitlements(tt.granted), withLock).Admit(t.Context(), tt.obj, write)
			if tt.wantRefused {
				if _, ok := ComputeLimitRefusal(err); !ok {
					t.Fatalf("Admit() error = %v, want a refusal", err)
				}
			} else if err != nil {
				t.Fatalf("Admit() error = %v", err)
			}

			stored := tt.obj.DeepCopyObject().(kclient.Object)
			written := storage.Get(t.Context(), kclient.ObjectKeyFromObject(tt.obj), stored) == nil
			if tt.update {
				written = stored.(*v1.VMCP).Spec.Manifest.Components == nil
			}
			if written == tt.wantRefused {
				t.Errorf("written = %v, want %v", written, !tt.wantRefused)
			}
			if (locked == 1) != tt.wantLock {
				t.Errorf("locked %d times, want the lock %v", locked, tt.wantLock)
			}
		})
	}
}

func TestRaised(t *testing.T) {
	for _, tt := range []struct {
		name          string
		limit         *int64
		before, after int64
		want          bool
	}{
		{
			name:   "unlimited",
			before: 0,
			after:  1 << 40,
		},
		{
			name:   "within the limit",
			limit:  new(int64(400)),
			before: 200,
			after:  400,
		},
		{
			name:   "past the limit",
			limit:  new(int64(400)),
			before: 200,
			after:  401,
			want:   true,
		},
		{
			name:   "lowered but still past the limit",
			limit:  new(int64(400)),
			before: 600,
			after:  500,
		},
		{
			name:   "a limit of 0 refuses any raise",
			limit:  new(int64(0)),
			before: 0,
			after:  1,
			want:   true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := raised(tt.limit, tt.before, tt.after); got != tt.want {
				t.Fatalf("raised() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComputeLimitRefusal(t *testing.T) {
	const message = "There isn't room in the plan for this."
	for _, tt := range []struct {
		name        string
		err         error
		wantRefusal bool
	}{
		{
			name: "storage refused the write",
			err: fmt.Errorf("failed to apply: %w", &apierrors.StatusError{
				ErrStatus: metav1.Status{
					Code:    http.StatusPaymentRequired,
					Message: message,
				},
			}),
			wantRefusal: true,
		},
		{
			name: "storage failed for another reason",
			err:  apierrors.NewConflict(schema.GroupResource{}, "vmcp1", errors.New("conflict")),
		},
		{
			name: "no error",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			refusal, ok := ComputeLimitRefusal(tt.err)
			if ok != tt.wantRefusal {
				t.Fatalf("ComputeLimitRefusal() ok = %v, want %v", ok, tt.wantRefusal)
			}
			if ok && refusal != message {
				t.Errorf("ComputeLimitRefusal() = %q, want %q", refusal, message)
			}
		})
	}
}

func TestComputeCapacity(t *testing.T) {
	deployments := types.MCPCapacityInfo{
		Source:            types.CapacitySourceResourceQuota,
		CPURequested:      "250m",
		CPULimit:          "8",
		MemoryRequested:   "512Mi",
		MemoryLimit:       "16Gi",
		ActiveDeployments: 2,
	}

	for _, tt := range []struct {
		name    string
		granted types.Entitlements
		want    types.MCPCapacityInfo
	}{
		{
			name: "the entitlements' limits and what the servers request replace the cluster's",
			granted: types.Entitlements{
				CPUMillicores: new(int64(2000)),
				MemoryMiB:     new(int64(8192)),
			},
			want: types.MCPCapacityInfo{
				Source:            types.CapacitySourceEntitlements,
				CPURequested:      "1.5",
				CPULimit:          "2",
				MemoryRequested:   "600Mi",
				MemoryLimit:       "8Gi",
				ActiveDeployments: 2,
			},
		},
		{
			name: "one limit leaves the other unlimited",
			granted: types.Entitlements{
				MemoryMiB: new(int64(8192)),
			},
			want: types.MCPCapacityInfo{
				Source:            types.CapacitySourceEntitlements,
				CPURequested:      "1.5",
				MemoryRequested:   "600Mi",
				MemoryLimit:       "8Gi",
				ActiveDeployments: 2,
			},
		},
		{
			name: "a limit of 0 is shown",
			granted: types.Entitlements{
				CPUMillicores: new(int64(0)),
				MemoryMiB:     new(int64(0)),
			},
			want: types.MCPCapacityInfo{
				Source:            types.CapacitySourceEntitlements,
				CPURequested:      "1.5",
				CPULimit:          "0",
				MemoryRequested:   "600Mi",
				MemoryLimit:       "0",
				ActiveDeployments: 2,
			},
		},
		{
			name: "without memory or CPU limits the cluster's capacity stays",
			granted: types.Entitlements{
				Users: new(int64(10)),
			},
			want: deployments,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// One server that isn't deployed yet still counts.
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(
				testVMCP("vmcp1", testComponent("shared", types.RuntimeContainerized, false, types.MCPResourceRequests{
					CPU:    "1500m",
					Memory: "600Mi",
				})),
			).Build()
			capacity := NewComputeCapacity(fakeCapacity(deployments), testEntitlements(tt.granted), storage)

			got, err := capacity.GetCapacityInfo(t.Context())
			if err != nil {
				t.Fatalf("GetCapacityInfo() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("GetCapacityInfo() = %+v, want %+v", got, tt.want)
			}

			// A server's own capacity keeps what its deployments request.
			got, err = capacity.GetCapacityInfoForServers(t.Context(), []string{"server1"})
			if err != nil {
				t.Fatalf("GetCapacityInfoForServers() error = %v", err)
			}
			want := tt.want
			want.CPURequested, want.MemoryRequested = deployments.CPURequested, deployments.MemoryRequested
			if got != want {
				t.Fatalf("GetCapacityInfoForServers() = %+v, want %+v", got, want)
			}
		})
	}
}

func (f fakeCapacity) GetCapacityInfo(context.Context) (types.MCPCapacityInfo, error) {
	return types.MCPCapacityInfo(f), nil
}

func (f fakeCapacity) GetCapacityInfoForServers(context.Context, []string) (types.MCPCapacityInfo, error) {
	return types.MCPCapacityInfo(f), nil
}
