package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal/principaltest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	vmcpEveryone    = types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}
	vmcpEngineering = types.Subject{Type: types.SubjectTypeUser, ID: "2"}
)

func vmcpAdminForTest() user.Info {
	return principaltest.AsUser(&user.DefaultInfo{UID: "admin", Groups: []string{types.GroupAdmin}})
}

func vmcpProfileForTest(name string, subjects ...types.Subject) types.VMCPProfile {
	return types.VMCPProfile{Name: name, Subjects: subjects, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
}

func callVMCPUpdate(t *testing.T, storage *vmcpTestStorage, gatewayClient *gateway.Client, handler *VMCPHandler, id string, manifest types.VMCPManifest) (types.VMCP, error) {
	t.Helper()
	body, err := json.Marshal(manifest)
	require.NoError(t, err)
	request := httptest.NewRequest(http.MethodPut, "/api/vmcps/"+id, bytes.NewReader(body))
	request.SetPathValue("vmcp_id", id)
	recorder := httptest.NewRecorder()
	if err := handler.Update(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           vmcpAdminForTest(),
	}); err != nil {
		return types.VMCP{}, err
	}
	var response types.VMCP
	require.NoError(t, json.NewDecoder(recorder.Body).Decode(&response))
	return response, nil
}

func requireBadRequest(t *testing.T, err error, contains string) {
	t.Helper()
	var badRequest *types.ErrHTTP
	require.ErrorAs(t, err, &badRequest)
	require.Equal(t, http.StatusBadRequest, badRequest.Code)
	require.ErrorContains(t, err, contains)
}

func TestVMCPProfileIDsSurviveEdits(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	gatewayClient := newHandlerTestGateway(t)
	handler := NewVMCPHandler(nil, "", "")

	manifest := testVMCPManifest()
	manifest.Profiles = []types.VMCPProfile{
		vmcpProfileForTest("a", vmcpEngineering),
		vmcpProfileForTest("b", vmcpEveryone),
	}
	created := callVMCPCreate(t, storage, gatewayClient, handler, manifest, vmcpAdminForTest())
	a, b := created.Profiles[0].ID, created.Profiles[1].ID
	require.NotEmpty(t, a)
	require.NotEmpty(t, b)
	require.NotEqual(t, a, b)

	// Rename, reorder and edit permissions: the IDs follow the profiles.
	edited := created.VMCPManifest
	edited.Profiles = []types.VMCPProfile{created.Profiles[1], created.Profiles[0]}
	edited.Profiles[1].Name = "renamed"
	edited.Profiles[0].Permissions = types.VMCPProfilePermissions{AllowedComponents: map[string]types.VMCPComponentSet{created.Components[0].ID: {AllowedTools: nil}}}
	updated, err := callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, edited)
	require.NoError(t, err)
	require.Equal(t, b, updated.Profiles[0].ID)
	require.Equal(t, a, updated.Profiles[1].ID)
	require.Equal(t, "renamed", updated.Profiles[1].Name)

	// A profile sent without its ID is new, even with the old name.
	replaced := updated.VMCPManifest
	replaced.Profiles = []types.VMCPProfile{updated.Profiles[0], vmcpProfileForTest("renamed", vmcpEngineering)}
	updated, err = callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, replaced)
	require.NoError(t, err)
	require.Equal(t, b, updated.Profiles[0].ID)
	require.NotEmpty(t, updated.Profiles[1].ID)
	require.NotEqual(t, a, updated.Profiles[1].ID)

	// The client cannot invent an ID, or bring back a removed profile's.
	invented := updated.VMCPManifest
	invented.Profiles[1].ID = a
	_, err = callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, invented)
	requireBadRequest(t, err, "unknown ID")
}

func TestVMCPCreateRejectsClientProfileIDs(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	manifest := testVMCPManifest()
	manifest.Profiles = []types.VMCPProfile{vmcpProfileForTest("a", vmcpEngineering)}
	manifest.Profiles[0].ID = "chosen"
	body, err := json.Marshal(manifest)
	require.NoError(t, err)
	err = NewVMCPHandler(nil, "", "").Create(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/api/vmcps", bytes.NewReader(body)),
		Storage:        storage,
		GatewayClient:  newHandlerTestGateway(t),
		User:           vmcpAdminForTest(),
	})
	requireBadRequest(t, err, "server-assigned")
}

func TestVMCPCreateEnforcesProfileSubjectRules(t *testing.T) {
	// agent is the subject of an agent that exists, which every test case's gateway database has.
	agent := types.Subject{Type: types.SubjectTypeAgent, ID: "1"}
	for _, tc := range []struct {
		name     string
		profiles []types.VMCPProfile
		wantErr  string
	}{
		{
			// There is no rule about wildcards across profiles: several may name every user, as they always could.
			name: "two profiles for every user",
			profiles: []types.VMCPProfile{
				vmcpProfileForTest("a", vmcpEveryone),
				vmcpProfileForTest("b", vmcpEveryone),
			},
		},
		{
			name: "agent wildcard",
			profiles: []types.VMCPProfile{
				vmcpProfileForTest("a", vmcpEveryone),
				vmcpProfileForTest("b", types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}),
			},
			wantErr: "agents must be named by ID",
		},
		{
			// An agent named by several profiles gets the union of their grants, as a user does.
			name: "one agent in two profiles",
			profiles: []types.VMCPProfile{
				vmcpProfileForTest("a", agent),
				vmcpProfileForTest("b", agent),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gatewayClient := newHandlerTestGateway(t)
			created, err := gatewayClient.CreateAgent(t.Context(), gatewaytypes.Agent{DisplayName: "builder"})
			require.NoError(t, err)
			require.Equal(t, agent.ID, strconv.FormatUint(uint64(created.ID), 10))
			manifest := testVMCPManifest()
			manifest.Profiles = tc.profiles
			body, err := json.Marshal(manifest)
			require.NoError(t, err)
			err = NewVMCPHandler(nil, "", "").Create(api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        httptest.NewRequest(http.MethodPost, "/api/vmcps", bytes.NewReader(body)),
				Storage:        newVMCPTestStorage(vmcpCatalogEntryForTest("entry")),
				GatewayClient:  gatewayClient,
				User:           vmcpAdminForTest(),
			})
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			requireBadRequest(t, err, tc.wantErr)
		})
	}
}

// TestVMCPProfilesForEveryUser checks that a vMCP with several profiles for every user validates on every edit, and that
// adopting snapshots keeps every profile's ID.
func TestVMCPProfilesForEveryUser(t *testing.T) {
	entry := vmcpCatalogEntryForTest("entry")
	entry.Spec.Manifest.RemoteConfig = &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"}
	stored := func() *v1.VMCP {
		manifest := testVMCPManifest()
		manifest.Components[0].ID = "component"
		manifest.Components[0].CatalogEntry = types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest}
		manifest.Profiles = []types.VMCPProfile{
			vmcpProfileForTest("a", vmcpEveryone),
			vmcpProfileForTest("b", vmcpEveryone),
		}
		manifest.Profiles[0].ID, manifest.Profiles[1].ID = "vp1a", "vp1b"
		return &v1.VMCP{Name: "vmcp1everyone", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: manifest}}
	}
	for _, tc := range []struct {
		name string
		edit func(*types.VMCPManifest)
	}{
		{
			name: "renaming a profile",
			edit: func(manifest *types.VMCPManifest) { manifest.Profiles[0].Name = "renamed" },
		},
		{
			name: "changing a profile's subjects",
			edit: func(manifest *types.VMCPManifest) {
				manifest.Profiles[0].Subjects = append(manifest.Profiles[0].Subjects, vmcpEngineering)
			},
		},
		{
			name: "adding a profile",
			edit: func(manifest *types.VMCPManifest) {
				manifest.Profiles = append(manifest.Profiles, vmcpProfileForTest("c", vmcpEveryone))
			},
		},
		{
			name: "an unrelated edit",
			edit: func(manifest *types.VMCPManifest) { manifest.DisplayName = "Renamed" },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vmcp := stored()
			storage := newVMCPTestStorage(entry, vmcp)
			handler := NewVMCPHandler(nil, "", "")
			manifest := vmcp.Spec.Manifest
			tc.edit(&manifest)
			_, err := callVMCPUpdate(t, storage, newHandlerTestGateway(t), handler, vmcp.Name, manifest)
			require.NoError(t, err)
		})
	}

	vmcp := stored()
	storage := newVMCPTestStorage(entry, vmcp)
	request := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+vmcp.Name+"/trigger-update", nil)
	request.SetPathValue("vmcp_id", vmcp.Name)
	require.NoError(t, NewVMCPHandler(nil, "", "").TriggerUpdate(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storage,
		User:           vmcpAdminForTest(),
	}))
	var updated v1.VMCP
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &updated))
	require.Equal(t, []string{"vp1a", "vp1b"}, []string{updated.Spec.Manifest.Profiles[0].ID, updated.Spec.Manifest.Profiles[1].ID})
}

// Updates are written with the resource version read at the start of the request, so of two concurrent updates one
// wins and the other fails with a conflict rather than overwriting it.
func TestVMCPConcurrentUpdatesConflict(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	gatewayClient := newHandlerTestGateway(t)
	handler := NewVMCPHandler(nil, "", "")
	manifest := testVMCPManifest()
	manifest.Profiles = []types.VMCPProfile{vmcpProfileForTest("a", vmcpEngineering)}
	created := callVMCPCreate(t, storage, gatewayClient, handler, manifest, vmcpAdminForTest())

	key := kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: created.ID}
	var raced bool
	storage.onUpdate = func(obj kclient.Object) {
		if _, ok := obj.(*v1.VMCP); !ok || raced {
			return
		}
		raced = true
		// The other update is saved while this one is in flight.
		var winner v1.VMCP
		require.NoError(t, storage.Get(t.Context(), key, &winner))
		winner.Spec.Manifest.Profiles[0].Name = "winner"
		require.NoError(t, storage.WithWatch.Update(t.Context(), &winner))
	}

	loser := created.VMCPManifest
	loser.Profiles[0].Name = "loser"
	_, err := callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, loser)
	require.Error(t, err)
	var status apierrors.APIStatus
	require.True(t, errors.As(err, &status) && apierrors.IsConflict(err), "error = %v", err)

	var stored v1.VMCP
	require.NoError(t, storage.Get(t.Context(), key, &stored))
	require.Equal(t, "winner", stored.Spec.Manifest.Profiles[0].Name)
	require.Equal(t, created.Profiles[0].ID, stored.Spec.Manifest.Profiles[0].ID)
}
