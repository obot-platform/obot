package mcp

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

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

func testVMCP(name string, components ...types.VMCPComponent) *v1.VMCP {
	return &v1.VMCP{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Components: components,
			},
		},
	}
}

func testInstance(name, vmcpID string) *v1.VMCPInstance {
	return &v1.VMCPInstance{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{
				VMCPID: vmcpID,
			},
		},
	}
}

func deleted[T interface{ SetDeletionTimestamp(*metav1.Time) }](obj T) T {
	now := metav1.Now()
	obj.SetDeletionTimestamp(&now)
	return obj
}

func TestHostedUsage(t *testing.T) {
	shared := testComponent("shared", types.RuntimeNPX, false, types.MCPResourceRequests{})
	dedicated := testComponent("dedicated", types.RuntimeUVX, true, types.MCPResourceRequests{})
	remote := testComponent("remote", types.RuntimeRemote, true, types.MCPResourceRequests{})
	sized := testComponent("sized", types.RuntimeContainerized, true, types.MCPResourceRequests{
		CPU:    "500m",
		Memory: "1Gi",
	})

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
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HostedUsage(tt.objs)
			if err != nil {
				t.Fatalf("HostedUsage() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("HostedUsage() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
