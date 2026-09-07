package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type vmcpTestStorage struct {
	kclient.WithWatch
	next int
}

func (s *vmcpTestStorage) Create(ctx context.Context, obj kclient.Object, opts ...kclient.CreateOption) error {
	if obj.GetName() == "" {
		s.next++
		obj.SetName(fmt.Sprintf("%stest-%d", obj.GetGenerateName(), s.next))
	}
	return s.WithWatch.Create(ctx, obj, opts...)
}

func TestVMCPForceSingleUserRejectsUnauthorizedWrites(t *testing.T) {
	for _, tc := range []struct {
		name    string
		update  bool
		current bool
		desired bool
	}{
		{
			name:    "create with override",
			desired: true,
		},
		{
			name:    "enable override",
			update:  true,
			desired: true,
		},
		{
			name:    "disable override",
			update:  true,
			current: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storage := newVMCPTestStorage()
			vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{
				UserID:   "1",
				Manifest: types.VMCPManifest{ForceSingleUser: tc.current},
			}}
			if tc.update {
				if err := storage.Create(t.Context(), vmcp); err != nil {
					t.Fatal(err)
				}
			}
			body, err := json.Marshal(types.VMCPManifest{ForceSingleUser: tc.desired})
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/vmcps", bytes.NewReader(body))
			request.SetPathValue("vmcp_id", vmcp.Name)
			ctx := api.Context{Request: request, Storage: storage, User: &user.DefaultInfo{UID: "1", Groups: []string{types.GroupPowerUserPlus}}}
			if tc.update {
				err = NewVMCPHandler(nil).Update(ctx)
			} else {
				err = NewVMCPHandler(nil).Create(ctx)
			}
			if err == nil || !strings.Contains(err.Error(), "only administrators") {
				t.Fatalf("expected authorization denial before any configuration writes, got %v", err)
			}
			var list v1.VMCPList
			if err := storage.List(t.Context(), &list); err != nil {
				t.Fatal(err)
			}
			if !tc.update && len(list.Items) != 0 {
				t.Fatal("rejected create persisted a VMCP")
			}
			if tc.update && (len(list.Items) != 1 || list.Items[0].Spec.Manifest.ForceSingleUser != tc.current) {
				t.Fatal("rejected update changed the override")
			}
		})
	}
}

func TestVMCPHandlerCreateAppliesScopeAndDefaults(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	gatewayClient := newHandlerTestGateway(t)
	handler := vmcpHandlerForTest(t, storage)
	manifest := testVMCPManifest()
	manifest.Profiles = nil
	manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{Key: "TOKEN"}}

	created := callVMCPCreate(t, storage, gatewayClient, handler, manifest, &user.DefaultInfo{
		Name: "user-1", UID: "user-1", Groups: []string{types.GroupAPI},
	})
	if created.UserID != "user-1" {
		t.Fatalf("personal VMCP userID = %q, want user-1", created.UserID)
	}
	if len(created.Profiles) != 1 || !created.Profiles[0].AllowAllTools {
		t.Fatalf("unexpected default profiles: %#v", created.Profiles)
	}
	if got := created.Components[0].Configuration[0].Policy; got != types.VMCPConfigurationPolicyProhibited {
		t.Fatalf("default configuration policy = %q, want prohibited", got)
	}

	shared := callVMCPCreate(t, storage, gatewayClient, handler, testVMCPManifest(), &user.DefaultInfo{
		Name: "admin", UID: "admin", Groups: []string{types.GroupAdmin},
	})
	if shared.UserID != "" {
		t.Fatalf("administrator-created VMCP userID = %q, want shared VMCP", shared.UserID)
	}
}

func TestVMCPHandlerCreateStoresStaticConfigurationInCredential(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	gatewayClient := newHandlerTestGateway(t)
	manifest := testVMCPManifest()
	manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{
		{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed, Value: "secret-token"},
		{Key: "REGION", Policy: types.VMCPConfigurationPolicyFixed, Value: "us-east-1"},
		{Key: "USER_HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed},
	}

	created := callVMCPCreate(t, storage, gatewayClient, NewVMCPHandler(nil), manifest, &user.DefaultInfo{
		Name: "admin", UID: "admin", Groups: []string{types.GroupAdmin},
	})
	for _, policy := range created.Components[0].Configuration {
		if policy.Value != "" {
			t.Fatalf("configuration %q value was returned from the VMCP", policy.Key)
		}
	}

	credential, err := gatewayClient.RevealCredential(t.Context(),
		[]string{vmcpconfig.StaticConfigurationCredentialContext(created.ID)},
		vmcpconfig.ConfigurationCredentialName(),
	)
	if err != nil {
		t.Fatalf("reveal static configuration: %v", err)
	}
	want := map[string]string{
		vmcpconfig.ConfigurationKey(created.Components[0].ID, "TOKEN"):  "secret-token",
		vmcpconfig.ConfigurationKey(created.Components[0].ID, "REGION"): "us-east-1",
	}
	if got := fmt.Sprint(credential.Secrets); got != fmt.Sprint(want) {
		t.Fatalf("static configuration = %v, want %v", credential.Secrets, want)
	}
	if created.StaticConfigurationHash != utils.Digest(want) {
		t.Fatalf("static configuration hash = %q, want %q", created.StaticConfigurationHash, utils.Digest(want))
	}

	var stored v1.VMCP
	if err := storage.Get(t.Context(), kclient.ObjectKey{Name: created.ID, Namespace: system.DefaultNamespace}, &stored); err != nil {
		t.Fatalf("get stored VMCP: %v", err)
	}
	if stored.Spec.Manifest.Components[0].Configuration[0].Value != "" {
		t.Fatal("static configuration was persisted in the VMCP manifest")
	}
}

func TestVMCPHandlerListFiltersByProfileForAdministrators(t *testing.T) {
	visible := &v1.VMCP{
		Name:      "vmcp-visible",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Profiles: []types.VMCPProfile{{
					Name: "administrator",
					Subjects: []types.Subject{{
						Type: types.SubjectTypeUser,
						ID:   "admin",
					}},
				}},
			},
		},
	}
	hiddenShared := &v1.VMCP{
		Name:      "vmcp-hidden-shared",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			Manifest: types.VMCPManifest{
				Profiles: []types.VMCPProfile{{
					Name: "other-user",
					Subjects: []types.Subject{{
						Type: types.SubjectTypeUser,
						ID:   "other",
					}},
				}},
			},
		},
	}
	hiddenPersonal := &v1.VMCP{
		Name:      "vmcp-hidden-personal",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			UserID: "owner",
		},
	}
	storage := newVMCPTestStorage(visible, hiddenShared, hiddenPersonal)
	recorder := httptest.NewRecorder()
	err := NewVMCPHandler(nil).List(api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodGet, "/api/vmcps", nil),
		Storage:        storage,
		User: &user.DefaultInfo{
			Name:   "admin",
			UID:    "admin",
			Groups: []string{types.GroupAPI, types.GroupAdmin},
		},
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	var response types.VMCPList
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("VMCPs = %#v, want only the profile-matched VMCP", response.Items)
	}
	if response.Items[0].ID != visible.Name {
		t.Fatalf("visible VMCP ID = %q, want %q", response.Items[0].ID, visible.Name)
	}
}

func TestVMCPInstanceSelectionValidation(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, selection := range []types.VMCPToolSet{nil, {}, {"everything": []string{"echo"}}, {"everything": []string{"forbidden"}}} {
			t.Run(fmt.Sprintf("%s/%v", method, selection), func(t *testing.T) {
				vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
					Components: []types.VMCPComponent{{ID: "everything", Name: "everything", AllowedTools: []string{"echo"}}},
					Profiles:   []types.VMCPProfile{{Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "team"}}, AllowedTools: types.VMCPToolSet{"everything": []string{"echo"}}}},
				}}}
				instance := &v1.VMCPInstance{Name: "vmcpi1test", Namespace: system.DefaultNamespace, Spec: v1.VMCPInstanceSpec{
					UserID:   "1",
					Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
				}}
				storage := newVMCPTestStorage(vmcp)
				if method == http.MethodPut {
					if err := storage.Create(t.Context(), instance); err != nil {
						t.Fatal(err)
					}
				}
				body, err := json.Marshal(types.VMCPInstanceManifest{VMCPID: vmcp.Name, EnabledTools: selection})
				if err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest(method, "/api/vmcp-instances", bytes.NewReader(body))
				req.SetPathValue("vmcp_instance_id", instance.Name)
				ctx := api.Context{
					ResponseWriter: httptest.NewRecorder(),
					Request:        req,
					Storage:        storage,
					User:           &user.DefaultInfo{UID: "1", Extra: map[string][]string{"auth_provider_groups": {"team"}}},
				}
				if method == http.MethodPost {
					err = NewVMCPInstanceHandler().Create(ctx)
				} else {
					err = NewVMCPInstanceHandler().Update(ctx)
				}
				rejected := len(selection["everything"]) > 0 && selection["everything"][0] == "forbidden"
				if (err != nil) != rejected {
					t.Fatalf("selection %v: error = %v", selection, err)
				}
			})
		}
	}
}

func TestVMCPInstanceCreateIsIdempotentPerUserAndVMCP(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp-shared",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{{
			Name:          "default",
			Subjects:      []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			AllowAllTools: true,
		}}, Components: []types.VMCPComponent{{ID: "component", Name: "component"}}}},
	}
	storage := newVMCPTestStorage(vmcp)
	handler := NewVMCPInstanceHandler()
	u := &user.DefaultInfo{Name: "user-1", UID: "user-1", Groups: []string{types.GroupAPI}}
	manifest := types.VMCPInstanceManifest{VMCPID: vmcp.Name, EnabledTools: types.VMCPToolSet{"component": []string{"tool-a"}}}

	first := callVMCPInstanceCreate(t, storage, handler, manifest, u)
	second := callVMCPInstanceCreate(t, storage, handler, manifest, u)
	if first.ID != second.ID {
		t.Fatalf("idempotent create returned IDs %q and %q", first.ID, second.ID)
	}
	if !strings.HasPrefix(first.ID, system.VMCPInstancePrefix) {
		t.Fatalf("VMCP instance ID = %q, want prefix %q", first.ID, system.VMCPInstancePrefix)
	}

	var list v1.VMCPInstanceList
	if err := storage.List(t.Context(), &list); err != nil {
		t.Fatalf("list instances: %v", err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("instance count = %d, want 1", len(list.Items))
	}
	if !slices.Equal(list.Items[0].Finalizers, []string{v1.VMCPInstanceFinalizer}) {
		t.Fatalf("missing instance credential cleanup finalizer: %v", list.Items[0].Finalizers)
	}
}

func TestVMCPInstanceListHidesInstanceAfterProfileAccessLoss(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp-shared",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{{
			Name:     "someone-else",
			Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "other"}},
		}}}},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi-user-1",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			UserID:   "user-1",
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		},
	}
	storage := newVMCPTestStorage(vmcp, instance)
	request := httptest.NewRequest(http.MethodGet, "/api/vmcp-instances", nil)
	recorder := httptest.NewRecorder()
	err := NewVMCPInstanceHandler().List(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        storage,
		User:           &user.DefaultInfo{Name: "user-1", UID: "user-1", Groups: []string{types.GroupAPI}},
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	var response types.VMCPInstanceList
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Items) != 0 {
		t.Fatalf("instances = %#v, want no instances after profile access loss", response.Items)
	}
}

func TestVMCPInstanceConfigureStoresOnlyUserAllowedConfiguration(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp-shared",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
			Components: []types.VMCPComponent{{
				ID:   "component-id",
				Name: "component",
				Configuration: []types.VMCPConfigurationPolicy{
					{Key: "STATIC", Policy: types.VMCPConfigurationPolicyFixed},
					{Key: "HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed},
				},
			}},
			Profiles: []types.VMCPProfile{{
				Name: "default", Subjects: []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}}, AllowAllTools: true,
			}},
		}},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi-user-1",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{
			UserID:   "user-1",
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
		},
	}
	storage := newVMCPTestStorage(vmcp, instance)
	gatewayClient := newHandlerTestGateway(t)
	body, err := json.Marshal(types.VMCPConfiguration{Components: map[string]map[string]string{
		"component-id": {"HEADER": "user-secret"},
	}})
	if err != nil {
		t.Fatalf("marshal configuration: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		"/api/vmcp-instances/"+instance.Name+"/configure", bytes.NewReader(body))
	request.SetPathValue("vmcp_instance_id", instance.Name)
	err = NewVMCPInstanceHandler().Configure(api.Context{
		ResponseWriter: recorder,
		Request:        request,
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           &user.DefaultInfo{Name: "user-1", UID: "user-1", Groups: []string{types.GroupAPI}},
	})
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}
	credential, err := gatewayClient.RevealCredential(t.Context(),
		[]string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)},
		vmcpconfig.ConfigurationCredentialName(),
	)
	if err != nil {
		t.Fatalf("reveal instance configuration: %v", err)
	}
	wantKey := vmcpconfig.ConfigurationKey("component-id", "HEADER")
	if len(credential.Secrets) != 1 || credential.Secrets[wantKey] != "user-secret" {
		t.Fatalf("instance configuration = %v, want %s=user-secret", credential.Secrets, wantKey)
	}
	var updatedInstance v1.VMCPInstance
	if err := storage.Get(t.Context(), kclient.ObjectKeyFromObject(instance), &updatedInstance); err != nil {
		t.Fatalf("get updated VMCP instance: %v", err)
	}
	if got, want := updatedInstance.Annotations[v1.VMCPInstanceConfigurationSyncAnnotation], utils.Digest(credential.Secrets); got != want {
		t.Fatalf("configuration sync annotation = %q, want %q", got, want)
	}
}

func TestVMCPInstanceConfigureRejectsFixedConfiguration(t *testing.T) {
	manifest := testVMCPManifest()
	manifest.Components[0].ID = "component-id"
	manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{
		Key: "STATIC", Policy: types.VMCPConfigurationPolicyFixed,
	}}
	vmcp := &v1.VMCP{Name: "vmcp-shared", Namespace: system.DefaultNamespace, Spec: v1.VMCPSpec{Manifest: manifest}}
	instance := &v1.VMCPInstance{
		Name: "vmcpi-user-1", Namespace: system.DefaultNamespace,
		Spec: v1.VMCPInstanceSpec{UserID: "user-1", Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name}},
	}
	body, err := json.Marshal(types.VMCPConfiguration{Components: map[string]map[string]string{
		"component-id": {"STATIC": "override"},
	}})
	if err != nil {
		t.Fatalf("marshal configuration: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost,
		"/api/vmcp-instances/"+instance.Name+"/configure", bytes.NewReader(body))
	request.SetPathValue("vmcp_instance_id", instance.Name)
	err = NewVMCPInstanceHandler().Configure(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        newVMCPTestStorage(vmcp, instance),
		GatewayClient:  newHandlerTestGateway(t),
		User:           &user.DefaultInfo{Name: "user-1", UID: "user-1", Groups: []string{types.GroupAPI}},
	})
	if err == nil {
		t.Fatal("Configure() accepted fixed configuration")
	}
}

func newVMCPTestStorage(objects ...kclient.Object) *vmcpTestStorage {
	return &vmcpTestStorage{WithWatch: clientfake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(objects...).
		WithIndex(&v1.VMCPInstance{}, "spec.userID", func(object kclient.Object) []string {
			return []string{object.(*v1.VMCPInstance).Spec.UserID}
		}).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(object kclient.Object) []string {
			return []string{object.(*v1.VMCPInstance).Spec.Manifest.VMCPID}
		}).
		Build()}
}

func callVMCPCreate(t *testing.T, storage *vmcpTestStorage, gatewayClient *gateway.Client, handler *VMCPHandler, manifest types.VMCPManifest, u user.Info) types.VMCP {
	t.Helper()
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	recorder := httptest.NewRecorder()
	err = handler.Create(api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodPost, "/api/vmcps", bytes.NewReader(body)),
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           u,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	var response types.VMCP
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

func callVMCPInstanceCreate(t *testing.T, storage *vmcpTestStorage, handler *VMCPInstanceHandler, manifest types.VMCPInstanceManifest, u user.Info) types.VMCPInstance {
	t.Helper()
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	recorder := httptest.NewRecorder()
	err = handler.Create(api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodPost, "/api/vmcp-instances", bytes.NewReader(body)),
		Storage:        storage,
		User:           u,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	var response types.VMCPInstance
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

func testVMCPManifest() types.VMCPManifest {
	return types.VMCPManifest{
		DisplayName: "Example",
		Components: []types.VMCPComponent{{
			Name:                    "component",
			MCPCatalogID:            "catalog",
			MCPServerCatalogEntryID: "entry",
		}},
		Profiles: []types.VMCPProfile{},
	}
}

func vmcpCatalogEntryForTest(id string) *v1.MCPServerCatalogEntry {
	return &v1.MCPServerCatalogEntry{
		ObjectMeta: metav1.ObjectMeta{Name: id, Namespace: system.DefaultNamespace},
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName:   "catalog",
			Manifest:         types.MCPServerCatalogEntryManifest{Name: "Stored " + id, Runtime: types.RuntimeRemote},
			UnsupportedTools: []string{"unsupported"},
		},
	}
}

func vmcpHandlerForTest(t *testing.T, storage kclient.Client) *VMCPHandler {
	t.Helper()
	indexer := gocache.NewIndexer(gocache.MetaNamespaceKeyFunc, gocache.Indexers{
		"selectors":           func(any) ([]string, error) { return []string{"*"}, nil },
		"catalog-entry-names": func(any) ([]string, error) { return nil, nil },
	})
	if err := indexer.Add(&v1.AccessControlRule{
		ObjectMeta: metav1.ObjectMeta{Name: "allow", Namespace: system.DefaultNamespace},
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: "catalog",
			Manifest: types.AccessControlRuleManifest{
				Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "user-1"}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	return NewVMCPHandler(accesscontrolrule.NewAccessControlRuleHelper(indexer, storage))
}

func TestVMCPComponentSnapshots(t *testing.T) {
	entry := vmcpCatalogEntryForTest("entry")
	entry.Spec.Manifest.RemoteConfig = &types.RemoteCatalogConfig{StaticOAuthRequired: true, FixedURL: "https://example.com/mcp"}
	storage := newVMCPTestStorage(entry)
	handler := vmcpHandlerForTest(t, storage)
	gatewayClient := newHandlerTestGateway(t)
	u := &user.DefaultInfo{UID: "user-1"}
	manifest := testVMCPManifest()
	manifest.Components[0].MCPCatalogID = "forged-catalog"
	manifest.Components[0].CatalogEntry.Manifest.Name = "forged-snapshot"
	manifest.Components[0].SourceDigest = "forged-digest"
	manifest.Components[0].OAuthCredentialID = "forged-credential"
	created := callVMCPCreate(t, storage, gatewayClient, handler, manifest, u)
	component := created.Components[0]
	if component.OAuthCredentialID != system.MCPOAuthCredentialName(entry.Name) {
		t.Fatalf("incorrect OAuth reference: %q", component.OAuthCredentialID)
	}
	if component.MCPCatalogID != "catalog" || component.CatalogEntry.Manifest.Name != entry.Spec.Manifest.Name ||
		component.SourceDigest != utils.Digest(component.CatalogEntry) || len(component.CatalogEntry.UnsupportedTools) != 1 {
		t.Fatalf("snapshot was not loaded from storage: %#v", component)
	}

	entry.Spec.Manifest.Name = "Updated source"
	entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired = false
	if err := storage.Update(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	// An update needs only the entry ID and the stable component identity, not a snapshot or catalog ID.
	manifest.Components[0] = types.VMCPComponent{ID: component.ID, MCPServerCatalogEntryID: entry.Name}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "/api/vmcps/"+created.ID, bytes.NewReader(body))
	request.SetPathValue("vmcp_id", created.ID)
	if err := handler.Update(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storage,
		GatewayClient:  gatewayClient,
		User:           u,
	}); err != nil {
		t.Fatal(err)
	}
	var stored v1.VMCP
	if err := storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: created.ID}, &stored); err != nil {
		t.Fatal(err)
	}
	updated := stored.Spec.Manifest.Components[0]
	if updated.SourceDigest != component.SourceDigest || updated.CatalogEntry.Manifest.Name != component.CatalogEntry.Manifest.Name || updated.OAuthCredentialID != component.OAuthCredentialID {
		t.Fatalf("ordinary update refreshed the snapshot: %#v", updated)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/vmcps/"+created.ID+"/trigger-update", nil)
	request.SetPathValue("vmcp_id", created.ID)
	if err := handler.TriggerUpdate(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        request,
		Storage:        storage,
		User:           u,
	}); err != nil {
		t.Fatal(err)
	}
	if err := storage.Get(t.Context(), kclient.ObjectKeyFromObject(&stored), &stored); err != nil {
		t.Fatal(err)
	}
	updated = stored.Spec.Manifest.Components[0]
	if updated.OAuthCredentialID != "" {
		t.Fatal("non-static component retained an OAuth reference")
	}
	if updated.CatalogEntry.Manifest.Name != "Updated source" || updated.ID != component.ID || updated.SourceDigest == component.SourceDigest {
		t.Fatalf("update did not resolve the current snapshot: %#v", updated)
	}
	if err := storage.Delete(t.Context(), entry); err != nil {
		t.Fatal(err)
	}
	// Deleted sources remain editable, but cannot be upgraded.
	ctx := api.Context{ResponseWriter: httptest.NewRecorder(), Request: request, Storage: storage, User: u, GatewayClient: gatewayClient}
	if err := handler.TriggerUpdate(ctx); err == nil {
		t.Fatal("upgrade with missing source succeeded")
	}
	request = httptest.NewRequest(http.MethodPut, "/api/vmcps/"+created.ID, bytes.NewReader(body))
	request.SetPathValue("vmcp_id", created.ID)
	ctx.Request = request
	if err := handler.Update(ctx); err != nil {
		t.Fatalf("edit with missing source failed: %v", err)
	}
}

func TestVMCPRemovalPrunesProfileComponents(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	handler := vmcpHandlerForTest(t, storage)
	gatewayClient := newHandlerTestGateway(t)
	u := &user.DefaultInfo{UID: "user-1"}
	manifest := testVMCPManifest()
	second := manifest.Components[0]
	second.Name = "second"
	manifest.Components = append(manifest.Components, second)
	created := callVMCPCreate(t, storage, gatewayClient, handler, manifest, u)
	manifest = created.VMCPManifest
	removedID, keptID := manifest.Components[0].ID, manifest.Components[1].ID
	manifest.Components = manifest.Components[1:]
	manifest.Profiles = []types.VMCPProfile{
		{Name: "explicit", Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "user-1"}}, AllowedTools: types.VMCPToolSet{removedID: {"echo"}, keptID: {"*"}}},
		{Name: "all", Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: "user-1"}}, AllowAllTools: true, AllowedTools: types.VMCPToolSet{removedID: {"*"}}},
	}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "/api/vmcps/"+created.ID, bytes.NewReader(body))
	request.SetPathValue("vmcp_id", created.ID)
	if err := handler.Update(api.Context{ResponseWriter: httptest.NewRecorder(), Request: request, Storage: storage, GatewayClient: gatewayClient, User: u}); err != nil {
		t.Fatal(err)
	}
	var stored v1.VMCP
	if err := storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: created.ID}, &stored); err != nil {
		t.Fatal(err)
	}
	for i := range manifest.Profiles {
		delete(manifest.Profiles[i].AllowedTools, removedID)
		if len(manifest.Profiles[i].AllowedTools) == 0 {
			manifest.Profiles[i].AllowedTools = nil // Empty grants are omitted in storage JSON.
		}
	}
	if !reflect.DeepEqual(stored.Spec.Manifest.Profiles, manifest.Profiles) {
		t.Fatalf("profiles = %#v, want %#v", stored.Spec.Manifest.Profiles, manifest.Profiles)
	}
}

func TestVMCPTriggerUpdateValidatesAllComponentsBeforeSaving(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%t", missing), func(t *testing.T) {
			first, second := vmcpCatalogEntryForTest("entry"), vmcpCatalogEntryForTest("second")
			first.Spec.Manifest.RemoteConfig = &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"}
			second.Spec.Manifest.RemoteConfig = &types.RemoteCatalogConfig{FixedURL: "https://example.com/mcp"}
			storage := newVMCPTestStorage(first, second)
			handler := vmcpHandlerForTest(t, storage)
			u := &user.DefaultInfo{UID: "user-1"}
			manifest := testVMCPManifest()
			manifest.Components = append(manifest.Components, types.VMCPComponent{Name: "second", MCPServerCatalogEntryID: second.Name})
			created := callVMCPCreate(t, storage, newHandlerTestGateway(t), handler, manifest, u)
			first.Spec.Manifest.Name = "new snapshot"
			if err := storage.Update(t.Context(), first); err != nil {
				t.Fatal(err)
			}
			if missing {
				if err := storage.Delete(t.Context(), second); err != nil {
					t.Fatal(err)
				}
			} else {
				second.Spec.Manifest.Runtime = types.RuntimeNPX
				if err := storage.Update(t.Context(), second); err != nil {
					t.Fatal(err)
				}
			}
			request := httptest.NewRequest(http.MethodPost, "/api/vmcps/"+created.ID+"/trigger-update", nil)
			request.SetPathValue("vmcp_id", created.ID)
			if err := handler.TriggerUpdate(api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        request,
				Storage:        storage,
				User:           u,
			}); err == nil {
				t.Fatal("invalid upgrade succeeded")
			}
			var stored v1.VMCP
			if err := storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: created.ID}, &stored); err != nil {
				t.Fatal(err)
			}
			if utils.Digest(stored.Spec.Manifest) != utils.Digest(created.VMCPManifest) {
				t.Fatal("failed upgrade changed stored snapshots or policy")
			}
		})
	}
}

func TestVMCPComponentAccess(t *testing.T) {
	for _, tc := range []struct {
		name      string
		personal  bool
		catalog   string
		workspace bool
		missing   bool
		allowed   bool
	}{
		{
			name:     "accessible catalog",
			personal: true,
			catalog:  "catalog",
			allowed:  true,
		},
		{
			name:     "denied second component",
			personal: true,
			catalog:  "private",
		},
		{
			name:    "shared does not require catalog grant",
			catalog: "private",
			allowed: true,
		},
		{
			name:      "own workspace",
			personal:  true,
			workspace: true,
			allowed:   true,
		},
		{
			name:     "missing source",
			personal: true,
			missing:  true,
		},
	} {
		for _, update := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/update=%t", tc.name, update), func(t *testing.T) {
				first, second := vmcpCatalogEntryForTest("entry"), vmcpCatalogEntryForTest("second")
				second.Spec.MCPCatalogName = tc.catalog
				workspace := &v1.PowerUserWorkspace{
					ObjectMeta: metav1.ObjectMeta{Name: "workspace", Namespace: system.DefaultNamespace},
					Spec:       v1.PowerUserWorkspaceSpec{UserID: "user-1"},
				}
				if tc.workspace {
					second.Spec.PowerUserWorkspaceID = workspace.Name
				}
				storage := newVMCPTestStorage(first, second, workspace)
				if tc.missing {
					if err := storage.Delete(t.Context(), second); err != nil {
						t.Fatal(err)
					}
				}
				handler := vmcpHandlerForTest(t, storage)
				manifest := testVMCPManifest()
				manifest.Components = append(manifest.Components, types.VMCPComponent{
					MCPServerCatalogEntryID: second.Name,
					MCPCatalogID:            "catalog", // Must not grant access to an entry in a different scope.
				})
				u := &user.DefaultInfo{UID: "user-1"}
				if !tc.personal {
					u.Groups = []string{types.GroupAdmin}
				}
				body, err := json.Marshal(manifest)
				if err != nil {
					t.Fatal(err)
				}
				ctx := api.Context{
					ResponseWriter: httptest.NewRecorder(),
					Request:        httptest.NewRequest(http.MethodPost, "/api/vmcps", bytes.NewReader(body)),
					Storage:        storage,
					GatewayClient:  newHandlerTestGateway(t),
					User:           u,
				}
				if update {
					vmcp := &v1.VMCP{Name: "vmcp-existing", Namespace: system.DefaultNamespace}
					if tc.personal {
						vmcp.Spec.UserID = u.UID
					}
					if err := storage.Create(t.Context(), vmcp); err != nil {
						t.Fatal(err)
					}
					ctx.Request.SetPathValue("vmcp_id", vmcp.Name)
					err = handler.Update(ctx)
				} else {
					err = handler.Create(ctx)
				}
				if (err == nil) != tc.allowed {
					t.Fatalf("error = %v, want allowed=%t", err, tc.allowed)
				}
				if !tc.allowed {
					if !tc.missing && !strings.Contains(err.Error(), "access denied") {
						t.Fatalf("expected access denial, got %v", err)
					}
					var list v1.VMCPList
					if err := storage.List(t.Context(), &list); err != nil {
						t.Fatal(err)
					}
					if update {
						if len(list.Items) != 1 || len(list.Items[0].Spec.Manifest.Components) != 0 {
							t.Fatal("rejected update changed the VMCP")
						}
					} else if len(list.Items) != 0 {
						t.Fatal("rejected create persisted a VMCP")
					}
				}
			})
		}
	}
}
