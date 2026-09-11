package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type vmcpTestStorage struct {
	kclient.WithWatch
	next      int
	onCreate  func(kclient.Object)
	onUpdate  func(kclient.Object)
	updateErr error
}

func TestVMCPHandlerRejectsProhibitedRequiredConfiguration(t *testing.T) {
	for _, operation := range []string{"create", "update", "trigger update"} {
		t.Run(operation, func(t *testing.T) {
			entry := vmcpCatalogEntryForTest("entry")
			entry.Spec.Manifest.Config = []types.MCPConfig{{
				Key:      "TOKEN",
				Required: true,
			}}
			manifest := testVMCPManifest()
			manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{
				Key:    "TOKEN",
				Policy: types.VMCPConfigurationPolicyProhibited,
			}}
			storage := newVMCPTestStorage(entry)
			if operation != "create" {
				manifest.Components[0].ID = "component-id"
				manifest.Components[0].CatalogEntry.Manifest = entry.Spec.Manifest
				if err := storage.Create(t.Context(), &v1.VMCP{
					Name:      "vmcp1test",
					Namespace: system.DefaultNamespace,
					Spec:      v1.VMCPSpec{Manifest: manifest},
				}); err != nil {
					t.Fatal(err)
				}
			}
			storage.onCreate = func(kclient.Object) { t.Fatal("rejected request created a resource") }
			storage.onUpdate = func(kclient.Object) { t.Fatal("rejected request updated a resource") }
			body, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/api/vmcps", bytes.NewReader(body))
			request.SetPathValue("vmcp_id", "vmcp1test")
			ctx := api.Context{
				Request:       request,
				Storage:       storage,
				GatewayClient: newHandlerTestGateway(t),
				User:          &user.DefaultInfo{UID: "admin", Groups: []string{types.GroupAdmin}},
			}
			handler := NewVMCPHandler(nil)
			switch operation {
			case "create":
				err = handler.Create(ctx)
			case "update":
				err = handler.Update(ctx)
			case "trigger update":
				err = handler.TriggerUpdate(ctx)
			}
			var httpErr *types.ErrHTTP
			if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest || !strings.Contains(httpErr.Message, `required configuration "TOKEN" cannot be prohibited`) {
				t.Fatalf("expected 400 for prohibited required configuration, got %v", err)
			}
		})
	}
}

func (s *vmcpTestStorage) Create(ctx context.Context, obj kclient.Object, opts ...kclient.CreateOption) error {
	if obj.GetName() == "" {
		s.next++
		obj.SetName(fmt.Sprintf("%stest-%d", obj.GetGenerateName(), s.next))
	}
	if s.onCreate != nil {
		s.onCreate(obj)
	}
	return s.WithWatch.Create(ctx, obj, opts...)
}

func (s *vmcpTestStorage) Update(ctx context.Context, obj kclient.Object, opts ...kclient.UpdateOption) error {
	if s.onUpdate != nil {
		s.onUpdate(obj)
	}
	if s.updateErr != nil {
		return s.updateErr
	}
	return s.WithWatch.Update(ctx, obj, opts...)
}

func TestVMCPOwnerCanChangeComponentForceSingleUser(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	gatewayClient := newHandlerTestGateway(t)
	handler := vmcpHandlerForTest(t, storage)
	owner := &user.DefaultInfo{Name: "user-1", UID: "user-1", Groups: []string{types.GroupAPI}}
	manifest := testVMCPManifest()
	manifest.Components[0].ForceSingleUser = true
	created := callVMCPCreate(t, storage, gatewayClient, handler, manifest, owner)
	if !created.Components[0].ForceSingleUser {
		t.Fatal("owner could not create a single-user component")
	}
	key := kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: created.ID}
	for _, forceSingleUser := range []bool{false, true} {
		var current v1.VMCP
		if err := storage.Get(t.Context(), key, &current); err != nil {
			t.Fatal(err)
		}
		current.Spec.Manifest.Components[0].ForceSingleUser = forceSingleUser
		body, err := json.Marshal(current.Spec.Manifest)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPut, "/api/vmcps/"+created.ID, bytes.NewReader(body))
		request.SetPathValue("vmcp_id", created.ID)
		if err := handler.Update(api.Context{
			Request:        request,
			ResponseWriter: httptest.NewRecorder(),
			Storage:        storage,
			GatewayClient:  gatewayClient,
			User:           owner,
		}); err != nil {
			t.Fatal(err)
		}
		if err := storage.Get(t.Context(), key, &current); err != nil {
			t.Fatal(err)
		}
		if current.Spec.Manifest.Components[0].ForceSingleUser != forceSingleUser {
			t.Fatalf("override was not saved as %v", forceSingleUser)
		}
	}
}

func TestMigratedVMCPAllowsAdminToDisableComponentForceSingleUser(t *testing.T) {
	storage := newVMCPTestStorage()
	vmcp := &v1.VMCP{
		Name:      "vmcp1migrated",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{
			LegacySlug: "legacy-composite",
			Manifest: types.VMCPManifest{Components: []types.VMCPComponent{{
				ID: "one", Name: "component", MCPServerCatalogEntryID: "entry", ForceSingleUser: true,
			}},
			},
		},
	}
	if err := storage.Create(t.Context(), vmcp); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "/api/vmcps/"+vmcp.Name, strings.NewReader(`{"displayName":"Migrated vMCP","components":[{"id":"one","mcpServerCatalogEntryID":"entry","forceSingleUser":false}]}`))
	request.SetPathValue("vmcp_id", vmcp.Name)
	err := NewVMCPHandler(nil).Update(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		GatewayClient:  newHandlerTestGateway(t),
		Request:        request,
		Storage:        storage,
		User:           &user.DefaultInfo{UID: "admin", Groups: []string{types.GroupAdmin}},
	})
	if err != nil {
		t.Fatalf("administrator could not disable forceSingleUser: %v", err)
	}
	var persisted v1.VMCP
	if err := storage.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Spec.Manifest.Components[0].ForceSingleUser {
		t.Fatal("administrator update did not disable forceSingleUser")
	}
	if persisted.Spec.LegacySlug != vmcp.Spec.LegacySlug {
		t.Fatal("update changed the legacy connection ID")
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
	if len(created.Profiles) != 0 {
		t.Fatalf("personal VMCP profiles = %#v, want none", created.Profiles)
	}
	if got := created.Components[0].Configuration[0].Policy; got != types.VMCPConfigurationPolicyProhibited {
		t.Fatalf("default configuration policy = %q, want prohibited", got)
	}

	shared := callVMCPCreate(t, storage, gatewayClient, handler, manifest, &user.DefaultInfo{
		Name: "admin", UID: "admin", Groups: []string{types.GroupAdmin},
	})
	if shared.UserID != "" {
		t.Fatalf("administrator-created VMCP userID = %q, want shared VMCP", shared.UserID)
	}
	if len(shared.Profiles) != 1 || shared.Profiles[0].Name != "me" || !shared.Profiles[0].AllowAllTools {
		t.Fatalf("unexpected shared default profiles: %#v", shared.Profiles)
	}
	if len(shared.Profiles[0].Subjects) != 1 || shared.Profiles[0].Subjects[0] != (types.Subject{Type: types.SubjectTypeUser, ID: "admin"}) {
		t.Fatalf("unexpected shared default profile subjects: %#v", shared.Profiles[0].Subjects)
	}

	personal := callVMCPCreate(t, storage, gatewayClient, handler, testVMCPManifest(), &user.DefaultInfo{
		Name: "admin", UID: "user-1", Groups: []string{types.GroupAdmin},
	}, "?scope=personal")
	if personal.UserID != "user-1" {
		t.Fatalf("administrator-created personal VMCP userID = %q, want user-1", personal.UserID)
	}

	if len(personal.Profiles) != 0 {
		t.Fatalf("administrator-created personal VMCP profiles = %#v, want none", personal.Profiles)
	}

	for _, tc := range []struct {
		vmcpID  string
		creator string
	}{
		{
			vmcpID:  created.ID,
			creator: "user-1",
		},
		{
			vmcpID:  shared.ID,
			creator: "admin",
		},
		{
			vmcpID:  personal.ID,
			creator: "user-1",
		},
	} {
		var persisted v1.VMCP
		if err := storage.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: tc.vmcpID}, &persisted); err != nil {
			t.Fatal(err)
		}
		if persisted.Spec.CreatorUserID != tc.creator {
			t.Fatalf("VMCP %q creator = %q, want %q", tc.vmcpID, persisted.Spec.CreatorUserID, tc.creator)
		}
	}
}

func TestVMCPHandlerCreateStoresStaticConfigurationInCredential(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	gatewayClient := newHandlerTestGateway(t)
	storage.onCreate = func(obj kclient.Object) {
		if hash := obj.(*v1.VMCP).Spec.StaticConfigurationHash; hash != "" {
			t.Fatalf("static configuration hash was published before storing the credential: %q", hash)
		}
	}
	published := false
	updateAttempts := 0
	storage.onUpdate = func(obj kclient.Object) {
		vmcp := obj.(*v1.VMCP)
		updateAttempts++
		if updateAttempts == 1 {
			var latest v1.VMCP
			if err := storage.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), &latest); err != nil {
				t.Fatal(err)
			}
			latest.Annotations = map[string]string{"concurrent-update": "preserved"}
			if err := storage.WithWatch.Update(t.Context(), &latest); err != nil {
				t.Fatal(err)
			}
		}
		if vmcp.Spec.StaticConfigurationHash == "" {
			t.Fatal("static configuration hash was not published")
		}
		componentID := vmcp.Spec.Manifest.Components[0].ID
		if got := vmcp.Spec.ComponentStaticConfigurationHashes[componentID]; got != utils.Digest(map[string]string{"TOKEN": "secret-token", "REGION": "us-east-1"}) {
			t.Fatalf("unexpected component static configuration hash: %q", got)
		}
		credential, err := gatewayClient.RevealCredential(t.Context(),
			[]string{vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name)},
			vmcpconfig.ConfigurationCredentialName(),
		)
		if err != nil {
			t.Fatalf("static configuration credential was not stored before publishing its hash: %v", err)
		}
		if got := credential.Secrets[vmcpconfig.ConfigurationKey(vmcp.Spec.Manifest.Components[0].ID, "TOKEN")]; got != "secret-token" {
			t.Fatalf("static configuration credential TOKEN = %q, want secret-token", got)
		}
		published = true
	}
	manifest := testVMCPManifest()
	manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{
		{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed, Value: "secret-token"},
		{Key: "REGION", Policy: types.VMCPConfigurationPolicyFixed, Value: "us-east-1"},
		{Key: "USER_HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed},
	}

	created := callVMCPCreate(t, storage, gatewayClient, NewVMCPHandler(nil), manifest, &user.DefaultInfo{
		Name: "admin", UID: "admin", Groups: []string{types.GroupAdmin},
	})
	if !published {
		t.Fatal("static configuration hash was not published")
	}
	if updateAttempts != 2 {
		t.Fatalf("update attempts = %d, want 2", updateAttempts)
	}
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
	if stored.Annotations["concurrent-update"] != "preserved" {
		t.Fatal("concurrent update was overwritten")
	}
}

func TestVMCPHandlerUpdatePreservesStaticConfiguration(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	gatewayClient := newHandlerTestGateway(t)
	handler := NewVMCPHandler(nil)
	admin := &user.DefaultInfo{UID: "admin", Groups: []string{types.GroupAdmin}}
	manifest := testVMCPManifest()
	manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{
		{
			Key:    "TOKEN",
			Policy: types.VMCPConfigurationPolicyFixed,
			Value:  "old-token",
		},
		{
			Key:    "REGION",
			Policy: types.VMCPConfigurationPolicyFixed,
			Value:  "old-region",
		},
		{
			Key:    "USER",
			Policy: types.VMCPConfigurationPolicyFixed,
		},
	}
	created := callVMCPCreate(t, storage, gatewayClient, handler, manifest, admin)
	for _, token := range []string{"new-token", ""} {
		var stored v1.VMCP
		key := kclient.ObjectKey{Name: created.ID, Namespace: system.DefaultNamespace}
		if err := storage.Get(t.Context(), key, &stored); err != nil {
			t.Fatal(err)
		}
		manifest := stored.Spec.Manifest
		manifest.Components[0].Configuration[0].Value = token
		body, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPut, "/api/vmcps/"+created.ID, bytes.NewReader(body))
		request.SetPathValue("vmcp_id", created.ID)
		if err := handler.Update(api.Context{
			Request:        request,
			ResponseWriter: httptest.NewRecorder(),
			Storage:        storage,
			GatewayClient:  gatewayClient,
			User:           admin,
		}); err != nil {
			t.Fatal(err)
		}
		credential, err := gatewayClient.RevealCredential(t.Context(),
			[]string{vmcpconfig.StaticConfigurationCredentialContext(created.ID)},
			vmcpconfig.ConfigurationCredentialName(),
		)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{
			vmcpconfig.ConfigurationKey(manifest.Components[0].ID, "TOKEN"):  "new-token",
			vmcpconfig.ConfigurationKey(manifest.Components[0].ID, "REGION"): "old-region",
		}
		if !reflect.DeepEqual(credential.Secrets, want) {
			t.Fatalf("static configuration = %v, want %v", credential.Secrets, want)
		}
		if err := storage.Get(t.Context(), key, &stored); err != nil {
			t.Fatal(err)
		}
		if stored.Spec.StaticConfigurationHash != utils.Digest(want) {
			t.Fatal("static configuration hash does not reflect preserved values")
		}
	}
}

func TestVMCPHandlerCreateDefersCleanupWhenPublishingStaticConfigurationFails(t *testing.T) {
	storage := newVMCPTestStorage(vmcpCatalogEntryForTest("entry"))
	storage.updateErr = fmt.Errorf("update failed")
	gatewayClient := newHandlerTestGateway(t)
	manifest := testVMCPManifest()
	manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{{
		Key:    "TOKEN",
		Policy: types.VMCPConfigurationPolicyFixed,
		Value:  "secret-token",
	}}
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	err = NewVMCPHandler(nil).Create(api.Context{
		Request:       httptest.NewRequest(http.MethodPost, "/api/vmcps", bytes.NewReader(body)),
		Storage:       storage,
		GatewayClient: gatewayClient,
		User:          &user.DefaultInfo{UID: "admin", Groups: []string{types.GroupAdmin}},
	})
	if err == nil || !strings.Contains(err.Error(), "failed to publish VMCP static configuration") {
		t.Fatalf("Create() error = %v, want static configuration publication error", err)
	}
	var vmcps v1.VMCPList
	if err := storage.List(t.Context(), &vmcps); err != nil {
		t.Fatal(err)
	}
	if len(vmcps.Items) != 1 || vmcps.Items[0].DeletionTimestamp.IsZero() {
		t.Fatalf("failed creation was not marked for finalization: %#v", vmcps.Items)
	}
	if !slices.Equal(vmcps.Items[0].Finalizers, []string{v1.VMCPFinalizer}) {
		t.Fatalf("missing credential cleanup finalizer: %v", vmcps.Items[0].Finalizers)
	}
	credential, err := gatewayClient.RevealCredential(t.Context(),
		[]string{vmcpconfig.StaticConfigurationCredentialContext(system.VMCPPrefix + "test-1")},
		vmcpconfig.ConfigurationCredentialName(),
	)
	if err != nil || len(credential.Secrets) != 1 {
		t.Fatalf("credential should remain for controller cleanup: %v", err)
	}
}

func TestVMCPDeletesDeferCredentialCleanup(t *testing.T) {
	for _, tc := range []struct {
		name    string
		object  kclient.Object
		pathKey string
		delete  func(api.Context) error
	}{
		{
			name: "vMCP",
			object: &v1.VMCP{
				Name:       "vmcp",
				Namespace:  system.DefaultNamespace,
				Finalizers: []string{v1.VMCPFinalizer},
			},
			pathKey: "vmcp_id",
			delete:  NewVMCPHandler(nil).Delete,
		},
		{
			name: "vMCP instance",
			object: &v1.VMCPInstance{
				Name:       "instance",
				Namespace:  system.DefaultNamespace,
				Finalizers: []string{v1.VMCPInstanceFinalizer},
			},
			pathKey: "vmcp_instance_id",
			delete:  NewVMCPInstanceHandler().Delete,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			storage := newVMCPTestStorage(tc.object)
			request := httptest.NewRequest(http.MethodDelete, "/", nil)
			request.SetPathValue(tc.pathKey, tc.object.GetName())
			// No gateway client: deletion must only mark the resource for finalization.
			if err := tc.delete(api.Context{Request: request, Storage: storage}); err != nil {
				t.Fatal(err)
			}
			if err := storage.Get(t.Context(), kclient.ObjectKeyFromObject(tc.object), tc.object); err != nil {
				t.Fatal(err)
			}
			if tc.object.GetDeletionTimestamp().IsZero() || len(tc.object.GetFinalizers()) == 0 {
				t.Fatal("resource was not retained for credential finalization")
			}
		})
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

func callVMCPCreate(t *testing.T, storage *vmcpTestStorage, gatewayClient *gateway.Client, handler *VMCPHandler, manifest types.VMCPManifest, u user.Info, query ...string) types.VMCP {
	t.Helper()
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	recorder := httptest.NewRecorder()
	err = handler.Create(api.Context{
		ResponseWriter: recorder,
		Request:        httptest.NewRequest(http.MethodPost, "/api/vmcps"+strings.Join(query, ""), bytes.NewReader(body)),
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
		Name: id, Namespace: system.DefaultNamespace,
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
		Name: "allow", Namespace: system.DefaultNamespace,
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
	u := &user.DefaultInfo{UID: "user-1", Groups: []string{types.GroupAdmin}}
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
					Name: "workspace", Namespace: system.DefaultNamespace,
					Spec: v1.PowerUserWorkspaceSpec{UserID: "user-1"},
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
					ctx.SetPathValue("vmcp_id", vmcp.Name)
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
