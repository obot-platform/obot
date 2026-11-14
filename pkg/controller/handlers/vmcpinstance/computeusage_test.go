package vmcpinstance

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	vmcphandler "github.com/obot-platform/obot/pkg/controller/handlers/vmcp"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// users looks up user 7, who is in no groups.
type users struct{}

func (users) UserInfoByID(context.Context, uint) (kuser.Info, error) {
	return &kuser.DefaultInfo{UID: "7"}, nil
}

// Compute is counted from vMCPs and their instances before any server exists, so it has to add up to what the
// servers the controllers create for them request.
func TestComputeUsageMatchesTheServersTheControllersCreate(t *testing.T) {
	component := func(id string, runtime types.Runtime, singleUser bool) types.VMCPComponent {
		manifest := types.MCPServerCatalogEntryManifest{
			Name:    id,
			Runtime: runtime,
		}
		switch runtime {
		case types.RuntimeNPX:
			manifest.NPXConfig = &types.NPXRuntimeConfig{Package: "@example/" + id}
		case types.RuntimeUVX:
			manifest.UVXConfig = &types.UVXRuntimeConfig{Package: id}
		case types.RuntimeRemote:
			manifest.RemoteConfig = &types.RemoteCatalogConfig{FixedURL: "https://example.com/" + id}
		}
		return types.VMCPComponent{
			ID:              id,
			Name:            id,
			ForceSingleUser: singleUser,
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{
				Manifest: manifest,
			},
		}
	}
	sized := component("sized", types.RuntimeUVX, true)
	sized.CatalogEntry.Manifest.Resources = &types.MCPResourceRequirements{
		Requests: types.MCPResourceRequests{
			CPU:    "500m",
			Memory: "1Gi",
		},
	}
	components := []types.VMCPComponent{
		component("shared", types.RuntimeNPX, false),
		component("dedicated", types.RuntimeUVX, true),
		sized,
		component("remote", types.RuntimeRemote, false),
		component("remote-dedicated", types.RuntimeRemote, true),
	}

	for _, tt := range []struct {
		name     string
		profiles []types.VMCPProfile
		// wantServers are the servers that run in a pod: shared ones once, dedicated ones for each of the two instances.
		wantServers int
		// wantOver is what's counted beyond what the created servers request.
		wantOver types.EntitlementsUsage
	}{
		{
			name:        "every component is enabled",
			profiles:    allowAllProfiles(),
			wantServers: 1 + 2*2,
		},
		{
			// Which components a profile enables depends on the user's groups, which change without a write to the
			// vMCP or its instances. Every dedicated component counts, here the sized one for both instances too,
			// so a group change can never take the environment past what was admitted.
			name: "a profile enables one dedicated component",
			profiles: []types.VMCPProfile{{
				Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
				Permissions: types.VMCPProfilePermissions{
					AllowedComponents: map[string]types.VMCPComponentSet{
						"dedicated": {},
					},
				},
			}},
			wantServers: 1 + 2*1,
			wantOver: types.EntitlementsUsage{
				MemoryMiB:     2 * 1024,
				CPUMillicores: 2 * 500,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			vmcp := &v1.VMCP{
				Name:      "vmcp1",
				Namespace: "default",
				Spec: v1.VMCPSpec{
					Manifest: types.VMCPManifest{
						Components: components,
						Profiles:   tt.profiles,
					},
				},
			}
			instance := func(name string) *v1.VMCPInstance {
				return &v1.VMCPInstance{
					Name:      name,
					Namespace: "default",
					Spec: v1.VMCPInstanceSpec{
						UserID: "7",
						Manifest: types.VMCPInstanceManifest{
							VMCPID: vmcp.Name,
						},
					},
				}
			}
			first, second := instance("vmcpi1first"), instance("vmcpi1second")
			client := withUserChangeWatches(t, fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(vmcp, first, second).
				WithIndex(&v1.MCPServer{}, "spec.vmcpID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPID} }).
				WithIndex(&v1.MCPServer{}, "spec.vmcpInstanceID", func(o kclient.Object) []string { return []string{o.(*v1.MCPServer).Spec.VMCPInstanceID} }), "7").Build()
			handler := &Handler{userInfo: users{}.UserInfoByID}

			// Create the servers as the controllers do.
			require.NoError(t, vmcphandler.EnsureMCPServers(router.Request{Ctx: t.Context(), Client: client, Object: vmcp}, nil))
			for _, instance := range []*v1.VMCPInstance{first, second} {
				require.NoError(t, handler.EnsureMCPServers(router.Request{Ctx: t.Context(), Client: client, Object: instance}, nil))
			}

			// Add up what the servers that run in a pod request.
			var servers v1.MCPServerList
			require.NoError(t, client.List(t.Context(), &servers))
			defaults := mcp.EffectiveDefaultMCPResourceRequirements(v1.K8sSettingsSpec{})
			var (
				created types.EntitlementsUsage
				pods    int
			)
			for _, server := range servers.Items {
				if server.Spec.Manifest.Runtime == types.RuntimeRemote {
					continue
				}
				pods++
				requests := defaults.Requests
				if resources, err := mcp.CoreResourceRequirements(server.Spec.Manifest.Resources); err != nil {
					t.Fatal(err)
				} else if resources != nil {
					requests = resources.Requests
				}
				created.MemoryMiB += requests.Memory().Value() / (1 << 20)
				created.CPUMillicores += requests.Cpu().MilliValue()
			}
			require.Equal(t, tt.wantServers, pods)

			compute, err := mcp.ListCompute(t.Context(), client)
			require.NoError(t, err)
			counted, err := compute.Usage()
			require.NoError(t, err)

			require.Equal(t, types.EntitlementsUsage{
				MemoryMiB:     created.MemoryMiB + tt.wantOver.MemoryMiB,
				CPUMillicores: created.CPUMillicores + tt.wantOver.CPUMillicores,
			}, counted)
		})
	}
}
