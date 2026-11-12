package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
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

// vmcpProfileDataForTest reports data for every profile once has is set, and otherwise for the profiles in ids. It
// counts the calls it gets.
type vmcpProfileDataForTest struct {
	has   bool
	ids   []string
	calls int
}

func (d *vmcpProfileDataForTest) ProfilesWithData(_ context.Context, _ string, profileIDs []string) ([]string, error) {
	d.calls++
	if d.has {
		return profileIDs, nil
	}
	var result []string
	for _, id := range profileIDs {
		if slices.Contains(d.ids, id) {
			result = append(result, id)
		}
	}
	return result, nil
}

func (*vmcpProfileDataForTest) DeleteProfileData(context.Context, string, []string) error {
	return nil
}

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
	handler := NewVMCPHandler(nil, "", "", nil)

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
	err = NewVMCPHandler(nil, "", "", nil).Create(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/api/vmcps", bytes.NewReader(body)),
		Storage:        storage,
		GatewayClient:  newHandlerTestGateway(t),
		User:           vmcpAdminForTest(),
	})
	requireBadRequest(t, err, "server-assigned")
}

func TestVMCPCreateEnforcesProfileSubjectRules(t *testing.T) {
	for _, tc := range []struct {
		name     string
		profiles []types.VMCPProfile
		wantErr  string
	}{
		{
			name: "two selector wildcards",
			profiles: []types.VMCPProfile{
				vmcpProfileForTest("a", vmcpEveryone),
				vmcpProfileForTest("b", vmcpEveryone),
			},
			wantErr: "both use the selector wildcard",
		},
		{
			name: "selector and agent wildcards",
			profiles: []types.VMCPProfile{
				vmcpProfileForTest("a", vmcpEveryone),
				vmcpProfileForTest("b", types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}),
			},
			wantErr: "not both",
		},
		{
			name: "one agent in two profiles",
			profiles: []types.VMCPProfile{
				vmcpProfileForTest("a", types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
				vmcpProfileForTest("b", types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
			},
			wantErr: "at most one profile",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := testVMCPManifest()
			manifest.Profiles = tc.profiles
			body, err := json.Marshal(manifest)
			require.NoError(t, err)
			err = NewVMCPHandler(nil, "", "", nil).Create(api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        httptest.NewRequest(http.MethodPost, "/api/vmcps", bytes.NewReader(body)),
				Storage:        newVMCPTestStorage(vmcpCatalogEntryForTest("entry")),
				GatewayClient:  newHandlerTestGateway(t),
				User:           vmcpAdminForTest(),
			})
			requireBadRequest(t, err, tc.wantErr)
		})
	}
}

// A vMCP stored with several selector wildcard profiles predates the wildcard rule. It keeps working, and validates on
// edits that leave every profile's subjects alone, until a profile has data stored for it.
func TestVMCPLegacyWildcardProfiles(t *testing.T) {
	entry := vmcpCatalogEntryForTest("entry")
	entry.Spec.Manifest.RemoteConfig = &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"}
	legacy := func() *v1.VMCP {
		manifest := testVMCPManifest()
		manifest.Components[0].ID = "component"
		manifest.Components[0].CatalogEntry = types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest}
		manifest.Profiles = []types.VMCPProfile{
			vmcpProfileForTest("a", vmcpEveryone),
			vmcpProfileForTest("b", vmcpEveryone),
		}
		manifest.Profiles[0].ID, manifest.Profiles[1].ID = "vp1a", "vp1b"
		return &v1.VMCP{Name: "vmcp1legacy", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: manifest}}
	}
	for _, tc := range []struct {
		name    string
		edit    func(*types.VMCPManifest)
		hasData bool
		wantErr string
	}{
		{
			name: "renaming a profile",
			edit: func(manifest *types.VMCPManifest) { manifest.Profiles[0].Name = "renamed" },
		},
		{
			name: "editing the display name",
			edit: func(manifest *types.VMCPManifest) { manifest.DisplayName = "Renamed" },
		},
		{
			name: "removing a wildcard profile",
			edit: func(manifest *types.VMCPManifest) { manifest.Profiles = manifest.Profiles[:1] },
		},
		{
			name: "changing a profile's subjects",
			edit: func(manifest *types.VMCPManifest) {
				manifest.Profiles[0].Subjects = append(manifest.Profiles[0].Subjects, vmcpEngineering)
			},
			wantErr: "both use the selector wildcard",
		},
		{
			name: "adding a profile",
			edit: func(manifest *types.VMCPManifest) {
				manifest.Profiles = append(manifest.Profiles, vmcpProfileForTest("c", vmcpEngineering))
			},
			wantErr: "both use the selector wildcard",
		},
		{
			name:    "an unrelated edit once a profile has data",
			edit:    func(manifest *types.VMCPManifest) { manifest.DisplayName = "Renamed" },
			hasData: true,
			wantErr: "profile configuration or OAuth grants",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := legacy()
			storage := newVMCPTestStorage(entry, stored)
			handler := NewVMCPHandler(nil, "", "", &vmcpProfileDataForTest{has: tc.hasData})
			manifest := stored.Spec.Manifest
			tc.edit(&manifest)
			_, err := callVMCPUpdate(t, storage, newHandlerTestGateway(t), handler, stored.Name, manifest)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			requireBadRequest(t, err, tc.wantErr)
		})
	}

	for _, hasData := range []bool{false, true} {
		stored := legacy()
		storage := newVMCPTestStorage(entry, stored)
		request := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+stored.Name+"/trigger-update", nil)
		request.SetPathValue("vmcp_id", stored.Name)
		err := NewVMCPHandler(nil, "", "", &vmcpProfileDataForTest{has: hasData}).TriggerUpdate(api.Context{
			ResponseWriter: httptest.NewRecorder(),
			Request:        request,
			Storage:        storage,
			User:           vmcpAdminForTest(),
		})
		if hasData {
			requireBadRequest(t, err, "profile configuration or OAuth grants")
			continue
		}
		require.NoError(t, err)
		// Adopting snapshots keeps every profile's ID.
		var updated v1.VMCP
		require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(stored), &updated))
		require.Equal(t, []string{"vp1a", "vp1b"}, []string{updated.Spec.Manifest.Profiles[0].ID, updated.Spec.Manifest.Profiles[1].ID})
	}
}

// Updates are written with the resource version read at the start of the request, so of two concurrent updates one
// wins and the other fails with a conflict rather than overwriting it.
func TestVMCPConcurrentUpdatesConflict(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	gatewayClient := newHandlerTestGateway(t)
	handler := NewVMCPHandler(nil, "", "", nil)
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

// A client that does not send profile IDs back must not silently drop a profile's configuration and OAuth grants, or
// pass them to a new profile of the same name.
func TestVMCPUpdateRefusesReplacingProfilesWithData(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	gatewayClient := newHandlerTestGateway(t)
	manifest := testVMCPManifest()
	manifest.Profiles = []types.VMCPProfile{
		vmcpProfileForTest("ops", vmcpEngineering),
		vmcpProfileForTest("dev", vmcpEngineering),
	}
	created := callVMCPCreate(t, storage, gatewayClient, NewVMCPHandler(nil, "", "", nil), manifest, vmcpAdminForTest())
	ops, dev := created.Profiles[0], created.Profiles[1]
	data := &vmcpProfileDataForTest{ids: []string{ops.ID}}
	handler := NewVMCPHandler(nil, "", "", data)

	// Sending the profiles back without their IDs is refused, after one check of the stored profiles they replace.
	withoutIDs := created.VMCPManifest
	withoutIDs.Profiles = []types.VMCPProfile{vmcpProfileForTest("ops", vmcpEngineering), vmcpProfileForTest("dev", vmcpEngineering)}
	_, err := callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, withoutIDs)
	requireBadRequest(t, err, `profile "ops" has stored configuration or OAuth grants; send its id`)
	require.Equal(t, 1, data.calls)

	// A profile without data may still be replaced by a new profile of its name.
	replaceDev := created.VMCPManifest
	replaceDev.Profiles = []types.VMCPProfile{ops, vmcpProfileForTest("dev", vmcpEngineering)}
	updated, err := callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, replaceDev)
	require.NoError(t, err)
	require.Equal(t, ops.ID, updated.Profiles[0].ID)
	require.NotEqual(t, dev.ID, updated.Profiles[1].ID)

	// Sending IDs behaves as before.
	renamed := updated.VMCPManifest
	renamed.Profiles[0].Name = "operations"
	updated, err = callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, renamed)
	require.NoError(t, err)
	require.Equal(t, ops.ID, updated.Profiles[0].ID)

	// A profile with data can be removed by leaving it out, and a later update may add a new profile of its name.
	removed := updated.VMCPManifest
	removed.Profiles = removed.Profiles[1:]
	updated, err = callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, removed)
	require.NoError(t, err)
	readded := updated.VMCPManifest
	readded.Profiles = append(readded.Profiles, vmcpProfileForTest("operations", vmcpEngineering))
	updated, err = callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, readded)
	require.NoError(t, err)
	require.NotEqual(t, ops.ID, updated.Profiles[1].ID)
}
