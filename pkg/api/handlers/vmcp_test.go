package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"k8s.io/apiserver/pkg/authentication/user"
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

func TestVMCPHandlerCreateAppliesScopeAndDefaults(t *testing.T) {
	storage := newVMCPTestStorage()
	gatewayClient := newHandlerTestGateway(t)
	handler := NewVMCPHandler()
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
	storage := newVMCPTestStorage()
	gatewayClient := newHandlerTestGateway(t)
	manifest := testVMCPManifest()
	manifest.Components[0].Configuration = []types.VMCPConfigurationPolicy{
		{Key: "TOKEN", Policy: types.VMCPConfigurationPolicyFixed, Value: "secret-token"},
		{Key: "REGION", Policy: types.VMCPConfigurationPolicyFixed, Value: "us-east-1"},
		{Key: "USER_HEADER", Policy: types.VMCPConfigurationPolicyUserAllowed},
	}

	created := callVMCPCreate(t, storage, gatewayClient, NewVMCPHandler(), manifest, &user.DefaultInfo{
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
	err := NewVMCPHandler().List(api.Context{
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

func TestVMCPInstanceCreateIsIdempotentPerUserAndVMCP(t *testing.T) {
	vmcp := &v1.VMCP{
		Name:      "vmcp-shared",
		Namespace: system.DefaultNamespace,
		Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{{
			Name:          "default",
			Subjects:      []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			AllowAllTools: true,
		}}}},
	}
	storage := newVMCPTestStorage(vmcp)
	handler := NewVMCPInstanceHandler()
	u := &user.DefaultInfo{Name: "user-1", UID: "user-1", Groups: []string{types.GroupAPI}}
	manifest := types.VMCPInstanceManifest{VMCPID: vmcp.Name, EnabledTools: []string{"tool-a"}}

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
