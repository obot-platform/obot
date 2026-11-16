package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/watch"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// watchingStorage stops a watch when its context ends, as the API server does and the fake client does not, and
// counts the watches it has started.
type watchingStorage struct {
	kclient.WithWatch
	watches atomic.Int32
}

func (s *watchingStorage) Watch(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) (watch.Interface, error) {
	w, err := s.WithWatch.Watch(ctx, list, opts...)
	if err != nil {
		return nil, err
	}
	s.watches.Add(1)
	go func() {
		<-ctx.Done()
		w.Stop()
	}()
	return w, nil
}

// newProfileConfigurationTestStorage returns storage that keeps vMCP status apart from the spec and checks resource
// versions on both, as the API server does, which profile configuration writes depend on.
func newProfileConfigurationTestStorage(objects ...kclient.Object) *vmcpTestStorage {
	return &vmcpTestStorage{WithWatch: &watchingStorage{WithWatch: clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.VMCP{}, &v1.VMCPInstance{}).
		WithObjects(objects...).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(object kclient.Object) []string {
			return []string{object.(*v1.VMCPInstance).Spec.UserID}
		}).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(object kclient.Object) []string {
			return []string{object.(*v1.VMCPInstance).Spec.Manifest.VMCPID}
		}).
		Build()}}
}

func watchesOf(storage *vmcpTestStorage) int32 {
	return storage.WithWatch.(*watchingStorage).watches.Load()
}

// profileConfigurationTestVMCP returns a stored vMCP whose component has a required and an optional value a user would
// enter and a value the vMCP fixes, with a profile for agent 7, one for every agent and one for every user.
func profileConfigurationTestVMCP() *v1.VMCP {
	profile := func(id string, subject types.Subject) types.VMCPProfile {
		return types.VMCPProfile{ID: id, Name: id, Subjects: []types.Subject{subject}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}}
	}
	return &v1.VMCP{Name: "vmcp1profiles", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		DisplayName: "Profiles",
		Components: []types.VMCPComponent{{
			ID:                      "api",
			Name:                    "api",
			MCPServerCatalogEntryID: "entry",
			CatalogEntry: types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Config: []types.MCPConfig{
				{Key: "REGION", Required: true},
				{Key: "VERBOSE"},
				{Key: "ADMIN_TOKEN", Required: true},
			}}},
			Configuration: []types.VMCPConfigurationPolicy{
				{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed},
				{Key: "VERBOSE", Policy: types.VMCPConfigurationPolicyUserAllowed},
				{Key: "ADMIN_TOKEN", Policy: types.VMCPConfigurationPolicyFixed},
			},
		}},
		Profiles: []types.VMCPProfile{
			profile("vp1seven", types.Subject{Type: types.SubjectTypeAgent, ID: "7"}),
			profile("vp1agents", types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}),
			profile("vp1users", types.Subject{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}),
		},
	}}}
}

// callProfileConfiguration calls a profile configuration route as an administrator, and returns the response body.
func callProfileConfiguration(t *testing.T, handler *VMCPProfileConfigurationHandler, storage *vmcpTestStorage, gatewayClient *gateway.Client, method, vmcpID, profileID string, body any) (string, error) {
	t.Helper()
	path := "/api/vmcps/" + vmcpID + "/profiles/" + profileID + "/configuration"
	call := handler.Update
	if method == http.MethodPost {
		path += "/reset"
		call = handler.Reset
	}
	var content []byte
	if body != nil {
		var err error
		content, err = json.Marshal(body)
		require.NoError(t, err)
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(content))
	request.SetPathValue("vmcp_id", vmcpID)
	request.SetPathValue("profile_id", profileID)
	recorder := httptest.NewRecorder()
	err := call(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           vmcpAdminForTest(),
	})
	return recorder.Body.String(), err
}

func requireHTTPStatus(t *testing.T, err error, code int) {
	t.Helper()
	var httpErr *types.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, code, httpErr.Code, httpErr.Message)
}

func TestVMCPProfileConfigurationAPI(t *testing.T) {
	vmcp := profileConfigurationTestVMCP()
	storage := newProfileConfigurationTestStorage(vmcp)
	gatewayClient := newHandlerTestGateway(t)
	handler := NewVMCPProfileConfigurationHandler(vmcpconfig.NewProfileConfigurationStore(gatewayClient))
	// No controller collects the reset's revisions here; TestVMCPProfileConfigurationResetWaitsForCollection has one.
	handler.collectionTimeout = 50 * time.Millisecond

	body, err := callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPut, vmcp.Name, "vp1seven", map[string]any{
		"components": map[string]any{"api": map[string]any{"REGION": "secret-region", "VERBOSE": "secret-verbose"}},
	})
	require.NoError(t, err)
	// The response names keys, never values.
	require.NotContains(t, body, "secret-")
	var status types.VMCPProfileStatus
	require.NoError(t, json.Unmarshal([]byte(body), &status))
	require.Equal(t, types.VMCPProfileStatus{
		ProfileID:    "vp1seven",
		AgentCapable: true,
		Components: []types.VMCPProfileComponentStatus{{
			ComponentID:   "api",
			Configuration: types.VMCPProfileConfigurationStatus{Set: []string{"REGION", "VERBOSE"}, MissingRequired: []string{}, OptionalUnset: []string{}},
		}},
	}, status)

	// The values are stored in the profile's own credential context.
	var stored v1.VMCP
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &stored))
	revision := stored.Status.ProfileConfigurationRevisions["vp1seven"].Name
	credential, err := gatewayClient.RevealCredential(t.Context(), []string{vmcpconfig.ProfileConfigurationCredentialContext(vmcp.Name, "vp1seven")}, revision)
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		vmcpconfig.ConfigurationKey("api", "REGION"):  "secret-region",
		vmcpconfig.ConfigurationKey("api", "VERBOSE"): "secret-verbose",
	}, credential.Secrets)
	// Nothing about the values is in the stored vMCP.
	storedJSON, err := json.Marshal(stored)
	require.NoError(t, err)
	require.NotContains(t, string(storedJSON), "secret-")

	// null clears a key.
	body, err = callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPut, vmcp.Name, "vp1seven", map[string]any{
		"components": map[string]any{"api": map[string]any{"VERBOSE": nil}},
	})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(body), &status))
	require.Equal(t, types.VMCPProfileConfigurationStatus{Set: []string{"REGION"}, MissingRequired: []string{}, OptionalUnset: []string{"VERBOSE"}}, status.Components[0].Configuration)

	// Values the vMCP fixes, and profiles that do not apply to agents, are refused.
	_, err = callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPut, vmcp.Name, "vp1seven", map[string]any{
		"components": map[string]any{"api": map[string]any{"ADMIN_TOKEN": "override"}},
	})
	requireHTTPStatus(t, err, http.StatusBadRequest)
	_, err = callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPut, vmcp.Name, "vp1users", map[string]any{
		"components": map[string]any{"api": map[string]any{"REGION": "us-east"}},
	})
	requireHTTPStatus(t, err, http.StatusBadRequest)
	_, err = callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPut, vmcp.Name, "vp1missing", map[string]any{"components": map[string]any{}})
	requireHTTPStatus(t, err, http.StatusNotFound)

	// Reset clears the profile's configuration.
	body, err = callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPost, vmcp.Name, "vp1seven", nil)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal([]byte(body), &status))
	require.Equal(t, types.VMCPProfileConfigurationStatus{Set: []string{}, MissingRequired: []string{"REGION"}, OptionalUnset: []string{"VERBOSE"}}, status.Components[0].Configuration)
}

// resetTestProfile configures a profile of a stored vMCP through the API and returns the handler and its store.
func resetTestProfile(t *testing.T, storage *vmcpTestStorage, gatewayClient *gateway.Client, vmcpID, profileID string) (*VMCPProfileConfigurationHandler, *vmcpconfig.ProfileConfigurationStore) {
	t.Helper()
	store := vmcpconfig.NewProfileConfigurationStore(gatewayClient)
	handler := NewVMCPProfileConfigurationHandler(store)
	_, err := callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPut, vmcpID, profileID, map[string]any{
		"components": map[string]any{"api": map[string]any{"REGION": "us-east"}},
	})
	require.NoError(t, err)
	return handler, store
}

func resetStatus(t *testing.T, body string) types.VMCPProfileStatus {
	t.Helper()
	var status types.VMCPProfileStatus
	require.NoError(t, json.Unmarshal([]byte(body), &status))
	return status
}

// TestVMCPProfileConfigurationResetWaitsForCollection checks that a reset answers once the vMCP controller has deleted
// the cleared revisions, so that the profile has no data left when it does.
func TestVMCPProfileConfigurationResetWaitsForCollection(t *testing.T) {
	vmcp := profileConfigurationTestVMCP()
	storage := newProfileConfigurationTestStorage(vmcp)
	gatewayClient := newHandlerTestGateway(t)
	handler, store := resetTestProfile(t, storage, gatewayClient, vmcp.Name, "vp1seven")

	// The vMCP controller, which no agent's instance keeps from it, collects the revisions once the reset is
	// published. It starts once the reset watches the vMCP, as the fake client sends no event from before a watch.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	collected := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				collected <- ctx.Err()
				return
			case <-ticker.C:
			}
			var current v1.VMCP
			if err := storage.Get(ctx, kclient.ObjectKeyFromObject(vmcp), &current); err != nil {
				collected <- err
				return
			}
			revision := current.Status.ProfileConfigurationRevisions["vp1seven"]
			if watchesOf(storage) == 0 || revision.Name != "" || len(revision.Superseded) == 0 {
				continue
			}
			changed, err := store.CollectSuperseded(ctx, &current, nil)
			if err == nil && changed {
				err = storage.Status().Update(ctx, &current)
			}
			collected <- err
			return
		}
	}()

	body, err := callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPost, vmcp.Name, "vp1seven", nil)
	require.NoError(t, err)
	status := resetStatus(t, body)
	require.False(t, status.PendingCollection, "the reset answered before its revisions were collected")
	require.Equal(t, []string{"REGION"}, status.Components[0].Configuration.MissingRequired)
	require.NoError(t, <-collected)
	withData, err := store.ProfilesWithData(t.Context(), vmcp.Name, []string{"vp1seven"})
	require.NoError(t, err)
	require.Empty(t, withData)
}

// TestVMCPProfileConfigurationResetTimesOut checks that a reset whose revisions are not collected in time still
// succeeds, and reports what is left to collect.
func TestVMCPProfileConfigurationResetTimesOut(t *testing.T) {
	vmcp := profileConfigurationTestVMCP()
	storage := newProfileConfigurationTestStorage(vmcp)
	gatewayClient := newHandlerTestGateway(t)
	handler, store := resetTestProfile(t, storage, gatewayClient, vmcp.Name, "vp1seven")
	handler.collectionTimeout = 100 * time.Millisecond

	body, err := callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPost, vmcp.Name, "vp1seven", nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, watchesOf(storage))
	status := resetStatus(t, body)
	require.True(t, status.PendingCollection)
	require.Equal(t, types.VMCPProfileConfigurationStatus{Set: []string{}, MissingRequired: []string{"REGION"}, OptionalUnset: []string{"VERBOSE"}}, status.Components[0].Configuration)
	var stored v1.VMCP
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &stored))
	require.Empty(t, stored.Status.ProfileConfigurationRevisions["vp1seven"].Name)
	// Agents are given nothing from it, but it counts as having data until it is collected.
	withData, err := store.ProfilesWithData(t.Context(), vmcp.Name, []string{"vp1seven"})
	require.NoError(t, err)
	require.Equal(t, []string{"vp1seven"}, withData)
}

// TestVMCPProfileConfigurationResetWithNothingToClear checks that a reset that clears nothing answers at once, whether
// the profile never had configuration or its last reset is still being collected.
func TestVMCPProfileConfigurationResetWithNothingToClear(t *testing.T) {
	vmcp := profileConfigurationTestVMCP()
	storage := newProfileConfigurationTestStorage(vmcp)
	gatewayClient := newHandlerTestGateway(t)
	handler, _ := resetTestProfile(t, storage, gatewayClient, vmcp.Name, "vp1seven")

	// A profile that never had configuration.
	handler.collectionTimeout = time.Minute
	body, err := callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPost, vmcp.Name, "vp1agents", nil)
	require.NoError(t, err)
	require.Zero(t, watchesOf(storage))
	require.False(t, resetStatus(t, body).PendingCollection)

	// A profile whose last reset is still being collected.
	handler.collectionTimeout = 50 * time.Millisecond
	_, err = callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPost, vmcp.Name, "vp1seven", nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, watchesOf(storage))
	handler.collectionTimeout = time.Minute
	body, err = callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPost, vmcp.Name, "vp1seven", nil)
	require.NoError(t, err)
	require.EqualValues(t, 1, watchesOf(storage))
	require.True(t, resetStatus(t, body).PendingCollection)
}

// TestVMCPUpdateGuardsProfileConfiguration checks the two vMCP update rules that depend on profile data with the real
// store: an update may not replace a profile that has configuration with a new profile of its name, and a vMCP stored
// before the wildcard rule loses its exception from it once a profile has configuration.
func TestVMCPUpdateGuardsProfileConfiguration(t *testing.T) {
	entry := vmcpCatalogEntryForTest("entry")
	entry.Spec.Manifest.Config = []types.MCPConfig{{Key: "REGION", Required: true}}
	gatewayClient := newHandlerTestGateway(t)
	store := vmcpconfig.NewProfileConfigurationStore(gatewayClient)
	configuration := NewVMCPProfileConfigurationHandler(store)
	handler := NewVMCPHandler(nil, "", "", vmcpconfig.ProfileDataStores{store})

	t.Run("replacing a profile", func(t *testing.T) {
		everyAgent := types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}
		storage := newProfileConfigurationTestStorage(entry.DeepCopy())
		manifest := testVMCPManifest()
		manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed}}
		manifest.Profiles = []types.VMCPProfile{vmcpProfileForTest("ops", everyAgent)}
		created := callVMCPCreate(t, storage, gatewayClient, handler, manifest, vmcpAdminForTest())
		ops := created.Profiles[0]

		// Without data, a client that does not send the ID back replaces the profile.
		withoutID := created.VMCPManifest
		withoutID.Profiles = []types.VMCPProfile{vmcpProfileForTest("ops", everyAgent)}
		updated, err := callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, withoutID)
		require.NoError(t, err)
		require.NotEqual(t, ops.ID, updated.Profiles[0].ID)
		ops = updated.Profiles[0]

		_, err = callProfileConfiguration(t, configuration, storage, gatewayClient, http.MethodPut, created.ID, ops.ID, map[string]any{
			"components": map[string]any{created.Components[0].ID: map[string]any{"REGION": "us-east"}},
		})
		require.NoError(t, err)
		withoutID = updated.VMCPManifest
		withoutID.Profiles = []types.VMCPProfile{vmcpProfileForTest("ops", everyAgent)}
		_, err = callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, withoutID)
		requireBadRequest(t, err, `profile "ops" has stored configuration or OAuth grants; send its id`)

		// Sending the ID keeps the profile, and its configuration.
		updated, err = callVMCPUpdate(t, storage, gatewayClient, handler, created.ID, updated.VMCPManifest)
		require.NoError(t, err)
		require.Equal(t, ops.ID, updated.Profiles[0].ID)
	})

	t.Run("legacy wildcard profiles", func(t *testing.T) {
		// Stored before the wildcard rule: one profile names everyone and another every user. Only the first names
		// agents, so agent selection is not ambiguous and it can be given configuration.
		manifest := testVMCPManifest()
		manifest.Components[0].ID = "api"
		manifest.Components[0].CatalogEntry = types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest}
		manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed}}
		manifest.Profiles = []types.VMCPProfile{
			vmcpProfileForTest("everyone", vmcpEveryone),
			vmcpProfileForTest("users", types.Subject{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}),
		}
		manifest.Profiles[0].ID, manifest.Profiles[1].ID = "vp1everyone", "vp1users"
		legacy := &v1.VMCP{Name: "vmcp1legacy", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: manifest}}
		require.Error(t, manifest.ValidateWildcardProfiles())
		storage := newProfileConfigurationTestStorage(entry.DeepCopy(), legacy)

		edited := manifest
		edited.DisplayName = "Renamed"
		_, err := callVMCPUpdate(t, storage, gatewayClient, handler, legacy.Name, edited)
		require.NoError(t, err)

		_, err = callProfileConfiguration(t, configuration, storage, gatewayClient, http.MethodPut, legacy.Name, "vp1everyone", map[string]any{
			"components": map[string]any{"api": map[string]any{"REGION": "us-east"}},
		})
		require.NoError(t, err)
		edited.DisplayName = "Renamed again"
		_, err = callVMCPUpdate(t, storage, gatewayClient, handler, legacy.Name, edited)
		requireBadRequest(t, err, "profile configuration or OAuth grants")
	})
}

// TestVMCPInstanceConfigurationRefusedForAgents checks that an agent's instance cannot be configured, deconfigured or
// revealed through the instance API, even by an administrator: profile configuration is its only source, and its
// values are never returned.
func TestVMCPInstanceConfigurationRefusedForAgents(t *testing.T) {
	vmcp := profileConfigurationTestVMCP()
	instance := &v1.VMCPInstance{Name: "vmcpi1agent", Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{
		UserID:   "agent:7",
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
	}}
	storage := newProfileConfigurationTestStorage(vmcp, instance)
	gatewayClient := newHandlerTestGateway(t)
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: vmcpconfig.InstanceConfigurationCredentialContext(instance.Name),
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: map[string]string{vmcpconfig.ConfigurationKey("api", "REGION"): "profile-secret"},
	}))
	handler := NewVMCPInstanceHandler()
	for _, tt := range []struct {
		action string
		call   func(api.Context) error
	}{
		{
			action: "configure",
			call:   handler.Configure,
		},
		{
			action: "reveal",
			call:   handler.Reveal,
		},
		{
			action: "deconfigure",
			call:   handler.Deconfigure,
		},
	} {
		t.Run(tt.action, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/vmcp-instances/"+instance.Name+"/"+tt.action, strings.NewReader(`{"components":{"api":{"REGION":"entered"}}}`))
			request.SetPathValue("vmcp_instance_id", instance.Name)
			recorder := httptest.NewRecorder()
			err := tt.call(api.Context{
				ResponseWriter: recorder,
				Request:        request,
				Storage:        storage,
				GatewayClient:  gatewayClient,
				User:           vmcpAdminForTest(),
			})
			requireHTTPStatus(t, err, http.StatusConflict)
			require.NotContains(t, recorder.Body.String(), "profile-secret")
		})
	}
	credential, err := gatewayClient.RevealCredential(t.Context(), []string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)}, vmcpconfig.ConfigurationCredentialName())
	require.NoError(t, err)
	require.Equal(t, map[string]string{vmcpconfig.ConfigurationKey("api", "REGION"): "profile-secret"}, credential.Secrets)

	// The instance API does not return the digest of an agent's values, which a user's instance still reports.
	instance.Status.UserConfigurationHash = "digest-of-the-values"
	require.Empty(t, convertVMCPInstance(*instance).Status.UserConfigurationHash)
	instance.Spec.UserID = "7"
	require.Equal(t, "digest-of-the-values", convertVMCPInstance(*instance).Status.UserConfigurationHash)
}

// TestVMCPProfileConfigurationOnCatalogVMCPs checks that a catalog-synced vMCP, whose manifest the API cannot change,
// can still be given profile configuration: nothing about it is in the manifest.
func TestVMCPProfileConfigurationOnCatalogVMCPs(t *testing.T) {
	vmcp := profileConfigurationTestVMCP()
	vmcp.Spec.SourceURL = "https://catalog.example.com/vmcps.yaml"
	storage := newProfileConfigurationTestStorage(vmcp)
	gatewayClient := newHandlerTestGateway(t)
	handler := NewVMCPProfileConfigurationHandler(vmcpconfig.NewProfileConfigurationStore(gatewayClient))
	_, err := callProfileConfiguration(t, handler, storage, gatewayClient, http.MethodPut, vmcp.Name, "vp1agents", map[string]any{
		"components": map[string]any{"api": map[string]any{"REGION": "us-east"}},
	})
	require.NoError(t, err)
	var stored v1.VMCP
	require.NoError(t, storage.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &stored))
	require.Equal(t, []string{vmcpconfig.ConfigurationKey("api", "REGION")}, stored.Status.ProfileConfigurationRevisions["vp1agents"].Keys)
	require.Equal(t, vmcp.Spec.Manifest, stored.Spec.Manifest)
}
